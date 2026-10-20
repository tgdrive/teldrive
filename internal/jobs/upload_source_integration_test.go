//go:build integration

package jobs_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/riverqueue/river"

	"github.com/tgdrive/teldrive/v2/internal/catalog"
	"github.com/tgdrive/teldrive/v2/internal/channels"
	"github.com/tgdrive/teldrive/v2/internal/jobs"
	"github.com/tgdrive/teldrive/v2/internal/telegramstore"
	testpostgres "github.com/tgdrive/teldrive/v2/internal/testutil/postgres"
	"github.com/tgdrive/teldrive/v2/internal/transfer"
	"github.com/tgdrive/teldrive/v2/internal/uploads"
)

func TestUploadSourceWorkerPublishesAndSkipsMatchingLocalFile(t *testing.T) {
	db := testpostgres.New(t)
	ctx := context.Background()
	seedCleanupOwner(t, db.Pool)
	root := t.TempDir()
	filePath := filepath.Join(root, "hello.txt")
	if err := os.WriteFile(filePath, []byte("hello uploader"), 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filePath)
	if err != nil {
		t.Fatal(err)
	}
	storage := &uploadWorkerStorage{}
	uploadService := uploads.NewService(db.Pool)
	catalogService := catalog.NewService(db.Pool, nil)
	channelService := channels.NewService(db.Pool, channels.TelegramCreator{Storage: storage}, channels.Config{PartLimit: 1000})
	pipeline := transfer.NewPipeline(uploadService, channelService, storage, nil, transfer.Config{})
	worker := jobs.NewUploadSourceWorker(db.Pool, catalogService, uploadService, pipeline, nil, 0, false)
	args := jobs.UploadSourceArgs{
		BatchID: "9ba4ddef-ea18-4a67-acd4-277f63d813ce", SourceIndex: 0, UserID: 1001, PartConcurrency: 2,
		Source: jobs.UploadFileSource{Type: "local", Location: filePath, DestinationPath: "folder/hello.txt", Size: info.Size(), ModTime: info.ModTime(), HasModTime: true},
	}
	job := &river.Job[jobs.UploadSourceArgs]{Args: args}
	if err := worker.Work(ctx, job); err != nil {
		t.Fatalf("first Work() error = %v", err)
	}
	if err := worker.Work(ctx, job); err != nil {
		t.Fatalf("second Work() error = %v", err)
	}
	if storage.uploads != 1 {
		t.Fatalf("Telegram uploads = %d, want 1", storage.uploads)
	}
	var count int
	var mimeType string
	if err := db.Pool.QueryRow(ctx, `SELECT count(*), min(f.mime_type) FROM files f JOIN files p ON p.id = f.parent_id WHERE f.name = 'hello.txt' AND p.name = 'folder' AND f.status = 'active'`).Scan(&count, &mimeType); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("published matching files = %d, want 1", count)
	}
	if mimeType != "text/plain" {
		t.Fatalf("published MIME type = %q, want text/plain", mimeType)
	}
}

type uploadWorkerStorage struct {
	mu          sync.Mutex
	uploads     int
	lastPayload []byte
}

func (s *uploadWorkerStorage) Upload(_ context.Context, request telegramstore.UploadRequest) (telegramstore.StoredPart, error) {
	payload, err := io.ReadAll(request.Reader)
	if err != nil {
		return telegramstore.StoredPart{}, err
	}
	if int64(len(payload)) != request.Size {
		return telegramstore.StoredPart{}, fmt.Errorf("payload size = %d, want %d", len(payload), request.Size)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.uploads++
	s.lastPayload = payload
	return telegramstore.StoredPart{ChannelID: request.ChannelID, MessageID: int64(s.uploads), Size: request.Size}, nil
}

func (*uploadWorkerStorage) OpenRange(context.Context, telegramstore.RangeRequest) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}
func (*uploadWorkerStorage) DeleteMessages(context.Context, int64, int64, []int64) error { return nil }
func (*uploadWorkerStorage) CopyPart(context.Context, int64, int64, int64, int64) (telegramstore.StoredPart, error) {
	return telegramstore.StoredPart{}, nil
}
func (*uploadWorkerStorage) CreateChannel(context.Context, int64, string) (telegramstore.Channel, error) {
	return telegramstore.Channel{}, nil
}
func (*uploadWorkerStorage) DeleteChannel(context.Context, int64, int64) error { return nil }

func TestUploadSourceEncryptionRequired(t *testing.T) {
	db := testpostgres.New(t)
	ctx := context.Background()
	seedCleanupOwner(t, db.Pool)
	payload := []byte("secret import")
	filePath := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(filePath, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filePath)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeContent(w, r, "secret.txt", time.Time{}, bytes.NewReader(payload))
	}))
	defer server.Close()
	storage := &uploadWorkerStorage{}
	uploadService := uploads.NewService(db.Pool)
	catalogService := catalog.NewService(db.Pool, nil)
	channelService := channels.NewService(db.Pool, channels.TelegramCreator{Storage: storage}, channels.Config{PartLimit: 1000})
	keys := transfer.StaticKeyProvider{1: "import-encryption-policy-test-key"}
	pipeline := transfer.NewPipeline(uploadService, channelService, storage, keys, transfer.Config{})
	for _, required := range []bool{false, true} {
		for _, requested := range []bool{false, true} {
			for _, sourceType := range []string{"local", "http"} {
				t.Run(fmt.Sprintf("required=%t/requested=%t/source=%s", required, requested, sourceType), func(t *testing.T) {
					name := uuid.NewString() + ".txt"
					location := filePath
					if sourceType == "http" {
						location = server.URL
					}
					// A plaintext session queued before policy activation must not be resumed
					// when encryption is now required.
					if required && !requested {
						mimeType := "text/plain"
						if _, err := uploadService.Create(ctx, uploads.CreateInput{UserID: 1001, Name: name, ExpectedSize: int64(len(payload)), MIMEType: &mimeType, ModTime: info.ModTime()}); err != nil {
							t.Fatal(err)
						}
					}
					worker := jobs.NewUploadSourceWorker(db.Pool, catalogService, uploadService, pipeline, server.Client(), 1, required)
					job := &river.Job[jobs.UploadSourceArgs]{Args: jobs.UploadSourceArgs{
						BatchID: uuid.NewString(), UserID: 1001, Encryption: requested,
						Source: jobs.UploadFileSource{Type: sourceType, Location: location, DestinationPath: name, Size: int64(len(payload)), ModTime: info.ModTime(), HasModTime: true},
					}}
					if err := worker.Work(ctx, job); err != nil {
						t.Fatalf("Work() = %v", err)
					}
					wantEncrypted := required || requested
					var encrypted bool
					var keyVersion int32
					if err := db.Pool.QueryRow(ctx, `SELECT encryption, COALESCE(encryption_key_version, 0) FROM files WHERE user_id=1001 AND name=$1 AND status='active'`, name).Scan(&encrypted, &keyVersion); err != nil {
						t.Fatal(err)
					}
					if encrypted != wantEncrypted || wantEncrypted && keyVersion != 1 || !wantEncrypted && keyVersion != 0 {
						t.Fatalf("file encryption=%t, key=%d, want encryption=%t", encrypted, keyVersion, wantEncrypted)
					}
					storage.mu.Lock()
					plaintext := bytes.Equal(storage.lastPayload, payload)
					storage.mu.Unlock()
					if plaintext == wantEncrypted {
						t.Fatalf("Telegram payload is plaintext=%t, want encryption=%t", plaintext, wantEncrypted)
					}
				})
			}
		}
	}
}
