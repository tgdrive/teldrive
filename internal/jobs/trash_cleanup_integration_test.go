//go:build integration

package jobs_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/riverqueue/river"

	"github.com/tgdrive/teldrive/v2/internal/jobs"
	testpostgres "github.com/tgdrive/teldrive/v2/internal/testutil/postgres"
)

func TestTrashCleanupWorkerPurgesOnlyExpiredTrashedRoots(t *testing.T) {
	db := testpostgres.New(t)
	ctx := context.Background()
	if _, err := db.Pool.Exec(ctx, "INSERT INTO users (user_id) VALUES (1001)"); err != nil {
		t.Fatal(err)
	}

	expiredRootID := uuid.New()
	expiredChildID := uuid.New()
	recentRootID := uuid.New()
	if _, err := db.Pool.Exec(ctx, `
INSERT INTO files (id, user_id, parent_id, name, normalized_name, kind, size, status, mod_time, deleted_at)
VALUES
    ($1, 1001, NULL, 'expired-root', 'expired-root', 'folder', NULL, 'trashed', now(), now() - interval '40 days'),
    ($2, 1001, $1, 'expired-child', 'expired-child', 'file', 0, 'trashed', now(), now() - interval '40 days'),
    ($3, 1001, NULL, 'recent-root', 'recent-root', 'file', 0, 'trashed', now(), now() - interval '2 days')
`, expiredRootID, expiredChildID, recentRootID); err != nil {
		t.Fatal(err)
	}

	service := &recordingPurgeService{}
	worker := jobs.NewTrashCleanupWorker(db.Pool, service)
	job := &river.Job[jobs.TrashCleanupSweepArgs]{Args: jobs.TrashCleanupSweepArgs{Retention: "720h", BatchSize: 10}}
	if err := worker.Work(ctx, job); err != nil {
		t.Fatalf("Work() error = %v", err)
	}
	calls := service.callsSnapshot()
	if len(calls) != 1 || calls[0].userID != 1001 || calls[0].fileID != expiredRootID {
		t.Fatalf("purge calls = %#v", calls)
	}

	if got := (jobs.TrashCleanupSweepArgs{}).Kind(); got != jobs.TrashCleanupSweepKind {
		t.Fatalf("Kind() = %q", got)
	}
	opts := (jobs.TrashCleanupSweepArgs{}).InsertOpts()
	if opts.Queue != jobs.CleanupQueue || opts.MaxAttempts != 3 || opts.Priority != 1 {
		t.Fatalf("InsertOpts() = %#v", opts)
	}
	if got := worker.Timeout(job); got != 30*time.Minute {
		t.Fatalf("Timeout() = %s", got)
	}
}

func TestTrashCleanupWorkerRejectsInvalidRetention(t *testing.T) {
	db := testpostgres.New(t)
	ctx := context.Background()
	worker := jobs.NewTrashCleanupWorker(db.Pool, &recordingPurgeService{})
	job := &river.Job[jobs.TrashCleanupSweepArgs]{Args: jobs.TrashCleanupSweepArgs{Retention: "not-a-duration"}}
	if err := worker.Work(ctx, job); err == nil {
		t.Fatal("Work() error = nil, want invalid retention")
	}
	if err := jobs.NewTrashCleanupWorker(db.Pool, nil).Work(ctx, job); !errors.Is(err, jobs.ErrTrashCleanupNotConfigured) {
		t.Fatalf("nil service error = %v", err)
	}
}

func TestRuntimePersistsTrashCleanupPeriodicJob(t *testing.T) {
	db := testpostgres.New(t)
	ctx := context.Background()
	runtime, err := jobs.NewRuntime(db.Pool, &cleanupStorage{}, &recordingPurgeService{})
	if err != nil {
		t.Fatalf("NewRuntime() error = %v", err)
	}
	if err := runtime.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer func() {
		if err := runtime.Stop(ctx); err != nil {
			t.Errorf("Stop() error = %v", err)
		}
	}()

	periodicJobs, err := runtime.ListPeriodicJobs(ctx)
	if err != nil {
		t.Fatalf("ListPeriodicJobs() error = %v", err)
	}
	for _, job := range periodicJobs {
		if job.Kind != jobs.TrashCleanupSweepKind {
			continue
		}
		if job.Schedule.CronExpression != "@every 1h" {
			t.Fatalf("trash cleanup schedule = %q", job.Schedule.CronExpression)
		}
		if got := string(job.Args["retention"]); got != `"720h"` {
			t.Fatalf("trash cleanup retention = %s", got)
		}
		if got := string(job.Args["batch_size"]); got != "100" {
			t.Fatalf("trash cleanup batch size = %s", got)
		}
		return
	}
	t.Fatal("trash cleanup periodic job not found")
}
