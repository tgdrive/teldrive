//go:build integration

package jobs

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/tgdrive/teldrive/v2/internal/telegramstore"
	testpostgres "github.com/tgdrive/teldrive/v2/internal/testutil/postgres"
)

func TestRuntimeAppliesProductionDefaults(t *testing.T) {
	db := testpostgres.New(t)
	runtime, err := NewRuntime(db.Pool, defaultsStorage{})
	if err != nil {
		t.Fatalf("NewRuntime() error = %v", err)
	}
	ctx := context.Background()
	if err := runtime.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if err := runtime.Stop(ctx); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
}

func TestRuntimePreservesPeriodicJobConfigurationOnRestart(t *testing.T) {
	db := testpostgres.New(t)
	ctx := context.Background()
	runtime, err := NewRuntime(db.Pool, defaultsStorage{})
	if err != nil {
		t.Fatalf("NewRuntime() error = %v", err)
	}
	if err := runtime.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	customCron := "15 3 * * *"
	job, err := runtime.UpdatePeriodicJob(ctx, cleanupPeriodicID, PeriodicJobInput{
		ID: cleanupPeriodicID, Kind: CleanupSweepKind,
		Args:  rawArgs(CleanupSweepArgs{BatchSize: 25}),
		Queue: CleanupQueue, Priority: 2, MaxAttempts: 3,
		Schedule: PeriodicSchedule{CronExpression: customCron, CronTimezone: maintenanceTimezone},
	})
	if err != nil {
		t.Fatalf("UpdatePeriodicJob() error = %v", err)
	}
	if job.Schedule.CronExpression != customCron {
		t.Fatalf("updated cron = %q, want %q", job.Schedule.CronExpression, customCron)
	}
	if err := runtime.Stop(ctx); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}

	restarted, err := NewRuntime(db.Pool, defaultsStorage{})
	if err != nil {
		t.Fatalf("NewRuntime() after restart error = %v", err)
	}
	if err := restarted.Start(ctx); err != nil {
		t.Fatalf("Start() after restart error = %v", err)
	}
	t.Cleanup(func() { _ = restarted.Stop(context.Background()) })

	jobs, err := restarted.ListPeriodicJobs(ctx)
	if err != nil {
		t.Fatalf("ListPeriodicJobs() error = %v", err)
	}
	for _, candidate := range jobs {
		if candidate.ID == cleanupPeriodicID {
			if candidate.Schedule.CronExpression != customCron {
				t.Fatalf("cron after restart = %q, want %q", candidate.Schedule.CronExpression, customCron)
			}
			return
		}
	}
	t.Fatalf("periodic job %q not found", cleanupPeriodicID)
}

type defaultsStorage struct{}

func (defaultsStorage) Upload(context.Context, telegramstore.UploadRequest) (telegramstore.StoredPart, error) {
	return telegramstore.StoredPart{}, errors.New("not used")
}
func (defaultsStorage) OpenRange(context.Context, telegramstore.RangeRequest) (io.ReadCloser, error) {
	return nil, errors.New("not used")
}
func (defaultsStorage) DeleteMessages(context.Context, int64, int64, []int64) error { return nil }
func (defaultsStorage) CopyPart(context.Context, int64, int64, int64, int64) (telegramstore.StoredPart, error) {
	return telegramstore.StoredPart{}, errors.New("not used")
}
func (defaultsStorage) CreateChannel(context.Context, int64, string) (telegramstore.Channel, error) {
	return telegramstore.Channel{}, errors.New("not used")
}
func (defaultsStorage) DeleteChannel(context.Context, int64, int64) error { return nil }
