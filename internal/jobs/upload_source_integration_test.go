//go:build integration

package jobs_test

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

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
	catalogService := catalog.NewService(db.Pool)
	channelService := channels.NewService(db.Pool, channels.TelegramCreator{Storage: storage}, channels.Config{PartLimit: 1000})
	pipeline := transfer.NewPipeline(uploadService, channelService, storage, nil, transfer.Config{})
	worker := jobs.NewUploadSourceWorker(db.Pool, catalogService, uploadService, pipeline, nil, 0)
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
	mu      sync.Mutex
	uploads int
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
