//go:build integration

package jobs

import (
	"context"
	"testing"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	testpostgres "github.com/tgdrive/teldrive/v2/internal/testutil/postgres"
)

func TestRuntimeRetryClearsCancellationMarker(t *testing.T) {
	db := testpostgres.New(t)
	runtime, err := NewRuntime(db.Pool, defaultsStorage{})
	if err != nil {
		t.Fatalf("NewRuntime() error = %v", err)
	}
	ctx := context.Background()
	inserted, err := runtime.client.Insert(ctx, UploadCleanupSweepArgs{}, &river.InsertOpts{
		Metadata: []byte(`{"custom":"kept"}`),
	})
	if err != nil {
		t.Fatalf("Insert() error = %v", err)
	}
	if _, err := runtime.Cancel(ctx, inserted.Job.ID); err != nil {
		t.Fatalf("Cancel() error = %v", err)
	}

	retried, err := runtime.Retry(ctx, inserted.Job.ID)
	if err != nil {
		t.Fatalf("Retry() error = %v", err)
	}
	if retried.State != string(rivertype.JobStateAvailable) {
		t.Fatalf("retry state = %q, want available", retried.State)
	}
	if _, ok := retried.Metadata["cancel_attempted_at"]; ok {
		t.Fatal("retry retained cancel_attempted_at metadata")
	}
	if got := string(retried.Metadata["custom"]); got != `"kept"` {
		t.Fatalf("custom metadata = %s, want kept", got)
	}

	persisted, err := runtime.Get(ctx, inserted.Job.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if _, ok := persisted.Metadata["cancel_attempted_at"]; ok {
		t.Fatal("persisted retry retained cancel_attempted_at metadata")
	}
}

func TestRuntimeRetryPreservesRunningCancellationMarker(t *testing.T) {
	db := testpostgres.New(t)
	runtime, err := NewRuntime(db.Pool, defaultsStorage{})
	if err != nil {
		t.Fatalf("NewRuntime() error = %v", err)
	}
	ctx := context.Background()
	inserted, err := runtime.client.Insert(ctx, UploadCleanupSweepArgs{}, nil)
	if err != nil {
		t.Fatalf("Insert() error = %v", err)
	}
	if _, err := db.Pool.Exec(ctx,
		"UPDATE river_job SET state = 'running', attempted_at = $2 WHERE id = $1",
		inserted.Job.ID, time.Now().UTC(),
	); err != nil {
		t.Fatalf("mark job running: %v", err)
	}
	if _, err := runtime.Cancel(ctx, inserted.Job.ID); err != nil {
		t.Fatalf("Cancel() error = %v", err)
	}

	retried, err := runtime.Retry(ctx, inserted.Job.ID)
	if err != nil {
		t.Fatalf("Retry() error = %v", err)
	}
	if retried.State != string(rivertype.JobStateRunning) {
		t.Fatalf("retry state = %q, want running", retried.State)
	}
	if _, ok := retried.Metadata["cancel_attempted_at"]; !ok {
		t.Fatal("retry removed active cancel_attempted_at metadata from running job")
	}
}
