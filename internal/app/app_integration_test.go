//go:build integration

package app_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	appapi "github.com/tgdrive/teldrive/v2/internal/api"
	"github.com/tgdrive/teldrive/v2/internal/app"
	"github.com/tgdrive/teldrive/v2/internal/config"
	"github.com/tgdrive/teldrive/v2/internal/telegramstore"
	testpostgres "github.com/tgdrive/teldrive/v2/internal/testutil/postgres"
)

func TestApplicationLifecycleAgainstRealPostgres(t *testing.T) {
	db := testpostgres.New(t)
	cfg := config.Default()
	cfg.Database.URL = db.URL
	cfg.HTTP.Address = "127.0.0.1:0"
	cfg.Telegram.AppID = 12345
	cfg.Telegram.AppHash = "telegram-app-hash"
	cfg.Security.SigningKey = "0123456789abcdef0123456789abcdef"
	cfg.Security.DataKey = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	cfg.HTTP.ShutdownTimeout = 10 * time.Second

	application, err := app.New(context.Background(), cfg, app.Dependencies{
		Storage: lifecycleStorage{}, Authenticator: lifecycleAuthenticator{}, Version: "integration",
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	serveResult := make(chan error, 1)
	go func() { serveResult <- application.Serve(ctx, listener) }()

	client := &http.Client{Timeout: 5 * time.Second}
	var response *http.Response
	for range 50 {
		response, err = client.Get("http://" + listener.Addr().String() + "/health/ready")
		if err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		cancel()
		t.Fatalf("health request failed: %v", err)
	}
	body, readErr := io.ReadAll(response.Body)
	response.Body.Close()
	if readErr != nil {
		t.Fatal(readErr)
	}
	if response.StatusCode != http.StatusOK {
		cancel()
		t.Fatalf("readiness status = %d, body=%s", response.StatusCode, body)
	}
	response, err = client.Get("http://" + listener.Addr().String() + "/api/health/ready")
	if err != nil {
		cancel()
		t.Fatalf("api health request failed: %v", err)
	}
	body, readErr = io.ReadAll(response.Body)
	response.Body.Close()
	if readErr != nil {
		cancel()
		t.Fatal(readErr)
	}
	if response.StatusCode != http.StatusOK {
		cancel()
		t.Fatalf("api readiness status = %d, body=%s", response.StatusCode, body)
	}

	cancel()
	select {
	case err := <-serveResult:
		if err != nil {
			t.Fatalf("Serve() error = %v", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("application did not stop")
	}
	if err := application.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
}

func TestApplicationInitializesFilesystemTelegramBackendWithoutCredentials(t *testing.T) {
	db := testpostgres.New(t)
	root := t.TempDir()
	cfg := config.Default()
	cfg.Database.URL = db.URL
	cfg.HTTP.Address = "127.0.0.1:0"
	cfg.Telegram.Backend = "filesystem"
	cfg.Telegram.LocalRoot = root
	cfg.Telegram.AppID = 0
	cfg.Telegram.AppHash = ""
	cfg.Security.SigningKey = "0123456789abcdef0123456789abcdef"
	cfg.Security.DataKey = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"

	application, err := app.New(context.Background(), cfg, app.Dependencies{
		Authenticator: lifecycleAuthenticator{}, Version: "integration",
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := application.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "state.json")); err != nil {
		t.Fatalf("local Telegram state was not created: %v", err)
	}
}

func TestNewRejectsInvalidConfiguration(t *testing.T) {
	t.Parallel()
	cfg := config.Default()
	if _, err := app.New(context.Background(), cfg, app.Dependencies{}); !errors.Is(err, config.ErrInvalid) {
		t.Fatalf("New() error = %v", err)
	}
}

type lifecycleAuthenticator struct{}

func (lifecycleAuthenticator) AuthenticateBearer(context.Context, string) (appapi.Identity, error) {
	return appapi.Identity{UserID: 1}, nil
}
func (lifecycleAuthenticator) AuthenticateAPIKey(context.Context, string) (appapi.Identity, error) {
	return appapi.Identity{UserID: 1}, nil
}

type lifecycleStorage struct{}

func (lifecycleStorage) Upload(context.Context, telegramstore.UploadRequest) (telegramstore.StoredPart, error) {
	return telegramstore.StoredPart{}, errors.New("not used")
}
func (lifecycleStorage) OpenRange(context.Context, telegramstore.RangeRequest) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(nil)), nil
}
func (lifecycleStorage) DeleteMessages(context.Context, int64, int64, []int64) error { return nil }
func (lifecycleStorage) CopyPart(context.Context, int64, int64, int64, int64) (telegramstore.StoredPart, error) {
	return telegramstore.StoredPart{}, errors.New("not implemented")
}
func (lifecycleStorage) CreateChannel(context.Context, int64, string) (telegramstore.Channel, error) {
	return telegramstore.Channel{}, errors.New("not used")
}
func (lifecycleStorage) DeleteChannel(context.Context, int64, int64) error { return nil }
