//go:build integration

package jobs

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/riverqueue/river"
	testpostgres "github.com/tgdrive/teldrive/v2/internal/testutil/postgres"
)

func TestRunPeriodicJobPreservesScheduleAndUsesSavedSettings(t *testing.T) {
	db := testpostgres.New(t)
	ctx := context.Background()
	runtime, err := NewRuntime(db.Pool, defaultsStorage{})
	if err != nil {
		t.Fatal(err)
	}
	for _, paused := range []bool{false, true} {
		id := "manual-active"
		if paused {
			id = "manual-paused"
		}
		_, err := runtime.CreatePeriodicJob(ctx, PeriodicJobInput{
			ID: id, Kind: FolderSizeRollupKind, Args: rawArgs(FolderSizeRollupArgs{}),
			Queue: CleanupQueue, Priority: 3, MaxAttempts: 7, Tags: []string{"manual"},
			Schedule: PeriodicSchedule{CronExpression: "0 0 1 1 *", CronTimezone: "UTC"},
			Paused:   paused,
		})
		if err != nil {
			t.Fatal(err)
		}
		before, err := runtime.client.PeriodicJobGet(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		job, err := runtime.RunPeriodicJob(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		stored, err := runtime.client.JobGet(ctx, job.ID)
		if err != nil {
			t.Fatal(err)
		}
		if stored.Kind != before.Kind || string(stored.EncodedArgs) != string(before.Args) ||
			stored.Queue != before.Queue || stored.Priority != before.Priority ||
			stored.MaxAttempts != before.MaxAttempts || !reflect.DeepEqual(stored.Tags, before.Tags) {
			t.Fatalf("queued job differs from saved settings: %#v", stored)
		}
		if string(stored.State) != "available" {
			t.Fatalf("job state = %s, want available", stored.State)
		}
		after, err := runtime.client.PeriodicJobGet(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(before, after) {
			t.Fatal("manual run changed periodic schedule or paused state")
		}
	}
	if _, err := runtime.RunPeriodicJob(ctx, "missing"); !errors.Is(err, river.ErrNotFound) {
		t.Fatalf("missing periodic job error = %v, want not found", err)
	}
}
