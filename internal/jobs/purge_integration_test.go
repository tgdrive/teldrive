//go:build integration

package jobs_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/riverqueue/river"

	"github.com/tgdrive/teldrive/v2/internal/jobs"
	testpostgres "github.com/tgdrive/teldrive/v2/internal/testutil/postgres"
)

func TestPendingFilePurgeWorkerProcessesDeletionPendingRoots(t *testing.T) {
	db := testpostgres.New(t)
	ctx := context.Background()
	if _, err := db.Pool.Exec(ctx, "INSERT INTO users (user_id) VALUES (1001)"); err != nil {
		t.Fatal(err)
	}
	rootID := uuid.New()
	childID := uuid.New()
	activeID := uuid.New()
	if _, err := db.Pool.Exec(ctx, `
INSERT INTO files (id, user_id, parent_id, name, normalized_name, kind, size, status, mod_time, deleted_at)
VALUES
    ($1, 1001, NULL, 'root', 'root', 'folder', NULL, 'deletion_pending', now(), now()),
    ($2, 1001, $1, 'child', 'child', 'file', 0, 'deletion_pending', now(), now()),
    ($3, 1001, NULL, 'active', 'active', 'file', 0, 'active', now(), NULL)
`, rootID, childID, activeID); err != nil {
		t.Fatal(err)
	}

	service := &recordingPurgeService{}
	worker := jobs.NewPendingFilePurgeWorker(db.Pool, service)
	job := &river.Job[jobs.PurgeSweepArgs]{Args: jobs.PurgeSweepArgs{}}
	if err := worker.Work(ctx, job); err != nil {
		t.Fatalf("Work() error = %v", err)
	}
	calls := service.callsSnapshot()
	if len(calls) != 1 || calls[0].userID != 1001 || calls[0].fileID != rootID {
		t.Fatalf("purge calls = %#v", calls)
	}

	if got := (jobs.PurgeSweepArgs{}).Kind(); got != jobs.PurgeSweepKind {
		t.Fatalf("Kind() = %q", got)
	}
	opts := (jobs.PurgeSweepArgs{}).InsertOpts()
	if opts.Queue != jobs.PurgeQueue || opts.MaxAttempts != 3 || opts.Priority != 1 {
		t.Fatalf("InsertOpts() = %#v", opts)
	}
	if got := worker.Timeout(job); got != 30*time.Minute {
		t.Fatalf("Timeout() = %s", got)
	}

	if err := (*jobs.PendingFilePurgeWorker)(nil).Work(ctx, job); !errors.Is(err, jobs.ErrPurgeNotConfigured) {
		t.Fatalf("nil worker error = %v", err)
	}
	if err := jobs.NewPendingFilePurgeWorker(db.Pool, nil).Work(ctx, job); !errors.Is(err, jobs.ErrPurgeNotConfigured) {
		t.Fatalf("nil service error = %v", err)
	}
}

func TestPendingFilePurgeWorkerReturnsServiceFailure(t *testing.T) {
	db := testpostgres.New(t)
	ctx := context.Background()
	if _, err := db.Pool.Exec(ctx, "INSERT INTO users (user_id) VALUES (1001)"); err != nil {
		t.Fatal(err)
	}
	fileID := uuid.New()
	if _, err := db.Pool.Exec(ctx, `
INSERT INTO files (id, user_id, name, normalized_name, kind, size, status, mod_time, deleted_at)
VALUES ($1, 1001, 'pending', 'pending', 'file', 0, 'deletion_pending', now(), now())
`, fileID); err != nil {
		t.Fatal(err)
	}
	serviceErr := errors.New("purge failed")
	worker := jobs.NewPendingFilePurgeWorker(db.Pool, &recordingPurgeService{err: serviceErr})
	if err := worker.Work(ctx, &river.Job[jobs.PurgeSweepArgs]{Args: jobs.PurgeSweepArgs{BatchSize: 1}}); !errors.Is(err, serviceErr) {
		t.Fatalf("Work() error = %v", err)
	}
}

type purgeCall struct {
	userID int64
	fileID uuid.UUID
}

type recordingPurgeService struct {
	mu    sync.Mutex
	calls []purgeCall
	err   error
}

func (s *recordingPurgeService) Purge(_ context.Context, userID int64, fileID uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, purgeCall{userID: userID, fileID: fileID})
	return s.err
}

func (s *recordingPurgeService) callsSnapshot() []purgeCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]purgeCall(nil), s.calls...)
}
