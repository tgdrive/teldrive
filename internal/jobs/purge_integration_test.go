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
	secondRootID := uuid.New()
	activeID := uuid.New()
	if _, err := db.Pool.Exec(ctx, `
INSERT INTO files (id, user_id, parent_id, name, normalized_name, kind, size, status, mod_time, deleted_at)
VALUES
    ($1, 1001, NULL, 'root', 'root', 'folder', NULL, 'deletion_pending', now(), now()),
    ($2, 1001, $1, 'child', 'child', 'file', 0, 'deletion_pending', now(), now()),
    ($3, 1001, NULL, 'second-root', 'second-root', 'file', 0, 'deletion_pending', now(), now()),
    ($4, 1001, NULL, 'active', 'active', 'file', 0, 'active', now(), NULL)
`, rootID, childID, secondRootID, activeID); err != nil {
		t.Fatal(err)
	}

	service := &recordingPurgeService{after: func(ctx context.Context, userID int64, fileID uuid.UUID) error {
		if _, err := db.Pool.Exec(ctx, "DELETE FROM files WHERE user_id = $1 AND parent_id = $2", userID, fileID); err != nil {
			return err
		}
		_, err := db.Pool.Exec(ctx, "DELETE FROM files WHERE user_id = $1 AND id = $2", userID, fileID)
		return err
	}}
	worker := jobs.NewPendingFilePurgeWorker(db.Pool, service)
	job := &river.Job[jobs.PurgeSweepArgs]{Args: jobs.PurgeSweepArgs{}}
	if err := worker.Work(ctx, job); err != nil {
		t.Fatalf("Work() error = %v", err)
	}
	calls := service.callsSnapshot()
	if len(calls) != 2 || calls[0].userID != 1001 || calls[1].userID != 1001 {
		t.Fatalf("purge calls = %#v", calls)
	}
	calledIDs := map[uuid.UUID]bool{calls[0].fileID: true, calls[1].fileID: true}
	if !calledIDs[rootID] || !calledIDs[secondRootID] {
		t.Fatalf("purge calls = %#v", calls)
	}
	if batches := service.batchesSnapshot(); len(batches) != 1 || batches[0] != 2 {
		t.Fatalf("purge batches = %v, want [2]", batches)
	}

	if got := (jobs.PurgeSweepArgs{}).Kind(); got != jobs.PurgeSweepKind {
		t.Fatalf("Kind() = %q", got)
	}
	opts := (jobs.PurgeSweepArgs{}).InsertOpts()
	if opts.Queue != jobs.PurgeQueue || opts.MaxAttempts != 3 || opts.Priority != 1 {
		t.Fatalf("InsertOpts() = %#v", opts)
	}
	if got := worker.Timeout(job); got != 2*time.Hour {
		t.Fatalf("Timeout() = %s", got)
	}

	if err := (*jobs.PendingFilePurgeWorker)(nil).Work(ctx, job); !errors.Is(err, jobs.ErrPurgeNotConfigured) {
		t.Fatalf("nil worker error = %v", err)
	}
	if err := jobs.NewPendingFilePurgeWorker(db.Pool, nil).Work(ctx, job); !errors.Is(err, jobs.ErrPurgeNotConfigured) {
		t.Fatalf("nil service error = %v", err)
	}
}

func TestPendingFilePurgeWorkerDrainsMultiplePages(t *testing.T) {
	db := testpostgres.New(t)
	ctx := context.Background()
	if _, err := db.Pool.Exec(ctx, "INSERT INTO users (user_id) VALUES (1001)"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Pool.Exec(ctx, `
INSERT INTO files (user_id, name, normalized_name, kind, size, status, mod_time, deleted_at)
SELECT 1001, 'pending-' || value, 'pending-' || value, 'file', 0, 'deletion_pending', now(), now()
FROM generate_series(1, 1001) AS value
`); err != nil {
		t.Fatal(err)
	}

	service := &recordingPurgeService{after: func(ctx context.Context, userID int64, fileID uuid.UUID) error {
		_, err := db.Pool.Exec(ctx, "DELETE FROM files WHERE user_id = $1 AND id = $2", userID, fileID)
		return err
	}}
	worker := jobs.NewPendingFilePurgeWorker(db.Pool, service)
	if err := worker.Work(ctx, &river.Job[jobs.PurgeSweepArgs]{Args: jobs.PurgeSweepArgs{}}); err != nil {
		t.Fatalf("Work() error = %v", err)
	}
	if calls := service.callsSnapshot(); len(calls) != 1001 {
		t.Fatalf("purge calls = %d, want 1001", len(calls))
	}
	if batches := service.batchesSnapshot(); len(batches) != 2 || batches[0] != 1000 || batches[1] != 1 {
		t.Fatalf("purge batches = %v, want [1000 1]", batches)
	}
	var remaining int
	if err := db.Pool.QueryRow(ctx, "SELECT count(*) FROM files WHERE status = 'deletion_pending'").Scan(&remaining); err != nil {
		t.Fatal(err)
	}
	if remaining != 0 {
		t.Fatalf("deletion-pending files = %d, want 0", remaining)
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
	if err := worker.Work(ctx, &river.Job[jobs.PurgeSweepArgs]{Args: jobs.PurgeSweepArgs{}}); !errors.Is(err, serviceErr) {
		t.Fatalf("Work() error = %v", err)
	}
}

type purgeCall struct {
	userID int64
	fileID uuid.UUID
}

type recordingPurgeService struct {
	mu      sync.Mutex
	calls   []purgeCall
	batches []int
	err     error
	after   func(context.Context, int64, uuid.UUID) error
}

func (s *recordingPurgeService) PurgeMany(ctx context.Context, userID int64, fileIDs []uuid.UUID) error {
	s.mu.Lock()
	s.batches = append(s.batches, len(fileIDs))
	s.mu.Unlock()
	for _, fileID := range fileIDs {
		if err := s.Purge(ctx, userID, fileID); err != nil {
			return err
		}
	}
	return nil
}

func (s *recordingPurgeService) Purge(ctx context.Context, userID int64, fileID uuid.UUID) error {
	s.mu.Lock()
	s.calls = append(s.calls, purgeCall{userID: userID, fileID: fileID})
	err, after := s.err, s.after
	s.mu.Unlock()
	if err != nil || after == nil {
		return err
	}
	return after(ctx, userID, fileID)
}

func (s *recordingPurgeService) callsSnapshot() []purgeCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]purgeCall(nil), s.calls...)
}

func (s *recordingPurgeService) batchesSnapshot() []int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]int(nil), s.batches...)
}
