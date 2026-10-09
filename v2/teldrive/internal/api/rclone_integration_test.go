//go:build integration

package api_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	api "github.com/tgdrive/teldrive/v2/internal/api"
	"github.com/tgdrive/teldrive/v2/internal/authn"
	"github.com/tgdrive/teldrive/v2/internal/catalog"
	"github.com/tgdrive/teldrive/v2/internal/channels"
	"github.com/tgdrive/teldrive/v2/internal/fileops"
	"github.com/tgdrive/teldrive/v2/internal/health"
	"github.com/tgdrive/teldrive/v2/internal/secureblob"
	"github.com/tgdrive/teldrive/v2/internal/shares"
	testpostgres "github.com/tgdrive/teldrive/v2/internal/testutil/postgres"
	"github.com/tgdrive/teldrive/v2/internal/transfer"
	"github.com/tgdrive/teldrive/v2/internal/uploads"
)

// Login methods are unused: this fixture authenticates a stored API key.
type rcloneLogin struct{ authn.TelegramLogin }

func TestRcloneV2FileLifecycle(t *testing.T) {
	binary := os.Getenv("TEST_RCLONE_BINARY")
	if binary == "" {
		t.Skip("TEST_RCLONE_BINARY is required for the optional CLI compatibility test")
	}
	db := testpostgres.New(t)
	ctx := context.Background()
	if _, err := db.Pool.Exec(ctx, "INSERT INTO users (user_id) VALUES (1001)"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Pool.Exec(ctx, "INSERT INTO channels (channel_id, user_id, name, selected) VALUES (9001, 1001, 'storage', true)"); err != nil {
		t.Fatal(err)
	}
	cipher, err := secureblob.NewWithKey(bytes.Repeat([]byte{5}, 32), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	auth, err := authn.NewService(db.Pool, cipher, &rcloneLogin{}, authn.Config{SigningKey: strings.Repeat("x", 32), Issuer: "rclone-test", AccessTokenTTL: time.Hour, RefreshTokenTTL: time.Hour, LoginFlowTTL: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	key, err := auth.CreateAPIKey(ctx, 1001, "rclone-compatibility", nil)
	if err != nil {
		t.Fatal(err)
	}
	catalogService := catalog.NewService(db.Pool, nil)
	uploadService := uploads.NewService(db.Pool)
	storage := &apiMemoryStorage{}
	channelService := channels.NewService(db.Pool, nil, channels.Config{PartLimit: 1000})
	fileService, err := fileops.NewService(db.Pool, catalogService, channelService, storage)
	if err != nil {
		t.Fatal(err)
	}
	shareService, err := shares.NewService(db.Pool, catalogService)
	if err != nil {
		t.Fatal(err)
	}
	pipeline := transfer.NewPipeline(uploadService, channelService, storage, nil, transfer.Config{})
	handler := api.NewHandler(catalogService, uploadService, pipeline, transfer.NewDownloader(catalogService, storage, nil), health.NewService("test", db.Pool), 0, nil).ConfigureDomains(auth, nil, channelService, fileService, shareService, nil)
	generated, err := api.NewServer(handler, api.NewSecurity(auth))
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.StripPrefix("/api", generated))
	t.Cleanup(server.Close)
	config := filepath.Join(t.TempDir(), "rclone.conf")
	if err := os.WriteFile(config, []byte("[drive]\ntype = teldrive\napi_host = "+server.URL+"\napi_key = "+key.Secret+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) string {
		t.Helper()
		commandCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
		defer cancel()
		command := exec.CommandContext(commandCtx, binary, append([]string{"--config", config, "--retries", "1", "--low-level-retries", "1"}, args...)...)
		output, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("rclone %v: %v\n%s", args, err, output)
		}
		return string(output)
	}
	run("mkdir", "drive:/compatibilidad")
	local := filepath.Join(t.TempDir(), "audio.txt")
	payload := "Prueba de rclone para Teldrive v2: ñ, audio y vídeo."
	if err := os.WriteFile(local, []byte(payload), 0600); err != nil {
		t.Fatal(err)
	}
	run("copyto", local, "drive:/compatibilidad/audio.txt")
	if got := run("cat", "drive:/compatibilidad/audio.txt"); got != payload {
		t.Fatalf("download mismatch: %q", got)
	}
	if got := run("lsf", "drive:/compatibilidad"); !strings.Contains(got, "audio.txt") {
		t.Fatalf("listing: %q", got)
	}
	run("moveto", "drive:/compatibilidad/audio.txt", "drive:/compatibilidad/renombrado.txt")
	run("copyto", "drive:/compatibilidad/renombrado.txt", "drive:/compatibilidad/copia.txt")
	if got := run("cat", "drive:/compatibilidad/copia.txt"); got != payload {
		t.Fatalf("copy mismatch: %q", got)
	}
	if got := run("link", "drive:/compatibilidad/copia.txt"); !strings.Contains(got, "/share/") {
		t.Fatalf("public link: %q", got)
	}
	run("deletefile", "drive:/compatibilidad/copia.txt")
	var trashed []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal([]byte(run("backend", "trash-list", "drive:")), &trashed); err != nil {
		t.Fatal(err)
	}
	if len(trashed) != 1 || trashed[0].Name != "copia.txt" {
		t.Fatalf("trash: %+v", trashed)
	}
	run("backend", "restore", "drive:", trashed[0].ID)
	if got := run("cat", "drive:/compatibilidad/copia.txt"); got != payload {
		t.Fatalf("restore mismatch: %q", got)
	}
	run("deletefile", "drive:/compatibilidad/copia.txt")
	run("backend", "purge", "drive:", trashed[0].ID)
	trashed = nil
	if err := json.Unmarshal([]byte(run("backend", "trash-list", "drive:")), &trashed); err != nil { t.Fatal(err) }
	if len(trashed) != 0 { t.Fatalf("purged trash: %+v", trashed) }
}
