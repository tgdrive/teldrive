//go:build integration

package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/google/uuid"

	api "github.com/tgdrive/teldrive/v2/internal/api"
	"github.com/tgdrive/teldrive/v2/internal/api/gen"
	"github.com/tgdrive/teldrive/v2/internal/catalog"
	"github.com/tgdrive/teldrive/v2/internal/health"
	testpostgres "github.com/tgdrive/teldrive/v2/internal/testutil/postgres"
	"github.com/tgdrive/teldrive/v2/internal/transfer"
	"github.com/tgdrive/teldrive/v2/internal/uploads"
)

func TestUploadEncryptionRequired(t *testing.T) {
	db := testpostgres.New(t)
	ctx := context.Background()
	if _, err := db.Pool.Exec(ctx, `
INSERT INTO users (user_id) VALUES (1001);
INSERT INTO channels (channel_id, user_id, name, selected) VALUES (9001, 1001, 'storage', true)`); err != nil {
		t.Fatal(err)
	}
	catalogService := catalog.NewService(db.Pool, nil)
	uploadService := uploads.NewService(db.Pool)
	storage := &apiMemoryStorage{}
	keys := transfer.StaticKeyProvider{1: "upload-encryption-policy-test-key"}
	pipeline := transfer.NewPipeline(uploadService, apiFixedResolver(9001), storage, keys, transfer.Config{})
	downloader := transfer.NewDownloader(catalogService, storage, keys)
	handler := api.NewHandler(catalogService, uploadService, pipeline, downloader, health.NewService("test", db.Pool), 1, nil)
	server, err := api.NewServer(handler, api.NewSecurity(apiAuthenticator{}))
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []bool{false, true} {
		for _, clientFlag := range []string{"omitted", "false", "true"} {
			for _, apiKey := range []bool{false, true} {
				t.Run(fmt.Sprintf("required=%t/client=%s/apiKey=%t", required, clientFlag, apiKey), func(t *testing.T) {
					handler.UploadEncryptionRequired = required
					headers := map[string]string{"Content-Type": "application/json", "Idempotency-Key": uuid.NewString()}
					if apiKey {
						headers["X-Api-Key"] = "test-key"
					} else {
						headers["Authorization"] = "Bearer test-token"
					}
					flag := ""
					if clientFlag != "omitted" {
						flag = `,"encryption":` + clientFlag
					}
					body := []byte(fmt.Sprintf(`{"name":%q,"size":6,"modTime":"2026-07-01T00:00:00Z"%s}`, uuid.NewString()+".txt", flag))
					created := performRequest(t, server, http.MethodPost, "/v1/uploads", body, headers)
					if created.Code != http.StatusCreated {
						t.Fatalf("create: %d %s", created.Code, created.Body.String())
					}
					var session gen.UploadSession
					if err := json.Unmarshal(created.Body.Bytes(), &session); err != nil {
						t.Fatal(err)
					}
					wantEncrypted := required || clientFlag == "true"
					if session.Encryption != wantEncrypted {
						t.Fatalf("encryption = %t, want %t", session.Encryption, wantEncrypted)
					}
					storedSession, err := uploadService.Get(ctx, 1001, uuid.UUID(session.ID))
					if err != nil {
						t.Fatal(err)
					}
					if storedSession.EncryptionKeyVersion.Valid != wantEncrypted || wantEncrypted && storedSession.EncryptionKeyVersion.Int32 != 1 {
						t.Fatalf("key version = %+v", storedSession.EncryptionKeyVersion)
					}
					path := "/v1/uploads/" + uuid.UUID(session.ID).String()
					headers["Content-Type"] = "application/octet-stream"
					part := performRequest(t, server, http.MethodPut, path+"/parts/1", []byte("secret"), headers)
					if part.Code != http.StatusCreated {
						t.Fatalf("put part: %d %s", part.Code, part.Body.String())
					}
					storage.mu.Lock()
					plaintext := bytes.Equal(storage.messages[storage.nextID], []byte("secret"))
					storage.mu.Unlock()
					if plaintext == wantEncrypted {
						t.Fatalf("Telegram payload is plaintext=%t, want encryption=%t", plaintext, wantEncrypted)
					}
					headers["Idempotency-Key"] = uuid.NewString()
					completed := performRequest(t, server, http.MethodPost, path+"/complete", nil, headers)
					if completed.Code != http.StatusCreated {
						t.Fatalf("complete: %d %s", completed.Code, completed.Body.String())
					}
					var file gen.FileEntry
					if err := json.Unmarshal(completed.Body.Bytes(), &file); err != nil {
						t.Fatal(err)
					}
					download := performRequest(t, server, http.MethodGet, "/v1/files/"+uuid.UUID(file.ID).String()+"/content", nil, headers)
					if download.Code != http.StatusOK || download.Body.String() != "secret" {
						t.Fatalf("download: %d %s", download.Code, download.Body.String())
					}
				})
			}
		}
	}
}
