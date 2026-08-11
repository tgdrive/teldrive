//go:build integration

package jobs_test

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/tgdrive/teldrive/v2/internal/dbtypes"
	"github.com/tgdrive/teldrive/v2/internal/jobs"
	"github.com/tgdrive/teldrive/v2/internal/telegramstore"
	testpostgres "github.com/tgdrive/teldrive/v2/internal/testutil/postgres"
	"github.com/tgdrive/teldrive/v2/internal/treehash"
	"github.com/tgdrive/teldrive/v2/internal/uploads"
)

func TestCleanupSweepExpiresAndDeletesTelegramParts(t *testing.T) {
	db := testpostgres.New(t)
	ctx := context.Background()
	seedCleanupOwner(t, db.Pool)
	catalog := uploads.NewService(db.Pool)
	session, err := catalog.Create(ctx, uploads.CreateInput{UserID: 1001, Name: "expired.bin", ExpectedSize: 1, PartSize: 1})
	if err != nil {
		t.Fatal(err)
	}
	uploadID, _ := dbtypes.GoogleUUID(session.ID)
	blockHashes, checksum := cleanupHash([]byte("x"))
	claim, err := catalog.ClaimPart(ctx, uploads.ClaimPartInput{
		UserID: 1001, UploadID: uploadID, PartNo: 1, ChannelID: 9001, PlainSize: 1, Checksum: &checksum,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := catalog.StorePart(ctx, uploads.StorePartInput{
		UploadID: uploadID, PartNo: 1, LeaseToken: claim.LeaseToken, MessageID: 77, StoredSize: 1,
		Checksum: checksum, BlockHashes: blockHashes,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Pool.Exec(ctx, "UPDATE upload_sessions SET expires_at = now() - interval '1 minute' WHERE id = $1", uploadID); err != nil {
		t.Fatal(err)
	}

	storage := &cleanupStorage{}
	worker := jobs.NewUploadCleanupWorker(db.Pool, storage)
	if err := worker.Work(ctx, &river.Job[jobs.CleanupSweepArgs]{Args: jobs.CleanupSweepArgs{BatchSize: 10}}); err != nil {
		t.Fatalf("Work() error = %v", err)
	}
	updated, err := catalog.Get(ctx, 1001, uploadID)
	if err != nil {
		t.Fatal(err)
	}
	if string(updated.State) != "expired" {
		t.Fatalf("state = %s", updated.State)
	}
	var partCount int
	if err := db.Pool.QueryRow(ctx, "SELECT count(*) FROM upload_parts WHERE upload_id = $1", uploadID).Scan(&partCount); err != nil {
		t.Fatal(err)
	}
	if partCount != 0 {
		t.Fatalf("remaining parts = %d", partCount)
	}
	if got := storage.deletedMessages(); len(got) != 1 || got[0] != 77 {
		t.Fatalf("deleted messages = %#v", got)
	}
}

func TestCleanupSweepRetainsReferencesWhenTelegramFails(t *testing.T) {
	db := testpostgres.New(t)
	ctx := context.Background()
	seedCleanupOwner(t, db.Pool)
	catalog := uploads.NewService(db.Pool)
	session, err := catalog.Create(ctx, uploads.CreateInput{UserID: 1001, Name: "aborted.bin", ExpectedSize: 1, PartSize: 1})
	if err != nil {
		t.Fatal(err)
	}
	uploadID, _ := dbtypes.GoogleUUID(session.ID)
	blockHashes, checksum := cleanupHash([]byte("x"))
	claim, err := catalog.ClaimPart(ctx, uploads.ClaimPartInput{UserID: 1001, UploadID: uploadID, PartNo: 1, ChannelID: 9001, PlainSize: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := catalog.StorePart(ctx, uploads.StorePartInput{
		UploadID: uploadID, PartNo: 1, LeaseToken: claim.LeaseToken, MessageID: 88, StoredSize: 1,
		Checksum: checksum, BlockHashes: blockHashes,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := catalog.Abort(ctx, 1001, uploadID); err != nil {
		t.Fatal(err)
	}

	storage := &cleanupStorage{deleteErr: errors.New("Telegram unavailable")}
	worker := jobs.NewUploadCleanupWorker(db.Pool, storage)
	if err := worker.Work(ctx, &river.Job[jobs.CleanupSweepArgs]{Args: jobs.CleanupSweepArgs{BatchSize: 10}}); err == nil {
		t.Fatal("expected Telegram cleanup failure")
	}
	var count int
	if err := db.Pool.QueryRow(ctx, "SELECT count(*) FROM upload_parts WHERE upload_id = $1", uploadID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("part count after failure = %d", count)
	}

	storage.mu.Lock()
	storage.deleteErr = nil
	storage.mu.Unlock()
	if err := worker.Work(ctx, &river.Job[jobs.CleanupSweepArgs]{Args: jobs.CleanupSweepArgs{BatchSize: 10}}); err != nil {
		t.Fatalf("retry Work() error = %v", err)
	}
	if err := db.Pool.QueryRow(ctx, "SELECT count(*) FROM upload_parts WHERE upload_id = $1", uploadID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("part count after retry = %d", count)
	}
}

func seedCleanupOwner(t testing.TB, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	if _, err := pool.Exec(ctx, "INSERT INTO users (user_id) VALUES (1001)"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO channels (channel_id, user_id, name, selected) VALUES (9001, 1001, 'storage', true)"); err != nil {
		t.Fatal(err)
	}
}

func cleanupHash(data []byte) ([]byte, string) {
	hasher := treehash.NewBlockHasher()
	_, _ = hasher.Write(data)
	blocks := hasher.Sum()
	return blocks, treehash.SumToHex(treehash.ComputeTreeHash(blocks))
}

type cleanupStorage struct {
	mu        sync.Mutex
	deleted   []int64
	deleteErr error
}

func (*cleanupStorage) Upload(context.Context, telegramstore.UploadRequest) (telegramstore.StoredPart, error) {
	return telegramstore.StoredPart{}, errors.New("not implemented")
}
func (*cleanupStorage) OpenRange(context.Context, telegramstore.RangeRequest) (io.ReadCloser, error) {
	return nil, errors.New("not implemented")
}
func (s *cleanupStorage) DeleteMessages(_ context.Context, _ int64, _ int64, ids []int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.deleteErr != nil {
		return s.deleteErr
	}
	s.deleted = append(s.deleted, ids...)
	return nil
}
func (*cleanupStorage) CopyPart(context.Context, int64, int64, int64, int64) (telegramstore.StoredPart, error) {
	return telegramstore.StoredPart{}, errors.New("not implemented")
}
func (*cleanupStorage) CreateChannel(context.Context, int64, string) (telegramstore.Channel, error) {
	return telegramstore.Channel{}, errors.New("not implemented")
}
func (*cleanupStorage) DeleteChannel(context.Context, int64, int64) error { return nil }
func (s *cleanupStorage) deletedMessages() []int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]int64(nil), s.deleted...)
}
