//go:build integration

package jobs_test

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/riverqueue/river"

	"github.com/tgdrive/teldrive/v2/internal/jobs"
	"github.com/tgdrive/teldrive/v2/internal/telegramstore"
	testpostgres "github.com/tgdrive/teldrive/v2/internal/testutil/postgres"
)

func TestOrphanCleanupDeletesOnlyExpiredUnreferencedDocuments(t *testing.T) {
	db := testpostgres.New(t)
	ctx := context.Background()
	seedCleanupOwner(t, db.Pool)
	if _, err := db.Pool.Exec(ctx, `
WITH session AS (
  INSERT INTO upload_sessions (user_id, name, normalized_name, expected_size, mod_time, part_size, expires_at)
  VALUES (1001, 'active.bin', 'active.bin', 1, now(), 1, now() + interval '7 days') RETURNING id
)
INSERT INTO upload_parts (upload_id, part_no, channel_id, message_id, plain_size, stored_size, state)
SELECT id, 1, 9001, 12, 1, 1, 'stored' FROM session`); err != nil {
		t.Fatal(err)
	}
	storage := &orphanStorage{messages: []telegramstore.DocumentMessage{
		{ID: 10, CreatedAt: time.Now().Add(-8 * 24 * time.Hour)},
		{ID: 11, CreatedAt: time.Now().Add(-6 * 24 * time.Hour)},
		{ID: 12, CreatedAt: time.Now().Add(-8 * 24 * time.Hour)},
	}}
	runtime, err := jobs.NewRuntime(db.Pool, storage)
	if err != nil {
		t.Fatal(err)
	}
	var template jobs.PeriodicTemplate
	for _, candidate := range runtime.PeriodicJobCatalog() {
		if candidate.Kind == jobs.OrphanCleanupKind {
			template = candidate
		}
	}
	if template.DefaultCronExpression != "@every 336h" || template.DefaultMaxAttempts != 3 || len(template.DefaultTags) != 0 {
		t.Fatalf("orphan cleanup template = %#v", template)
	}
	if _, err := runtime.CreatePeriodicJob(ctx, jobs.PeriodicJobInput{
		ID: template.ID, Kind: template.Kind, Args: template.DefaultArgs, Queue: template.DefaultQueue,
		Priority: template.DefaultPriority, MaxAttempts: template.DefaultMaxAttempts,
		Schedule: jobs.PeriodicSchedule{CronExpression: template.DefaultCronExpression, CronTimezone: template.DefaultCronTimezone},
	}); err != nil {
		t.Fatalf("CreatePeriodicJob() error = %v", err)
	}
	worker := jobs.NewOrphanedTelegramPartsCleanupWorker(db.Pool, storage, storage, 7*24*time.Hour)
	if err := worker.Work(ctx, &river.Job[jobs.OrphanCleanupArgs]{Args: jobs.OrphanCleanupArgs{PageSize: 100}}); err != nil {
		t.Fatal(err)
	}
	if len(storage.deleted) != 1 || storage.deleted[0] != 10 {
		t.Fatalf("deleted messages = %v, want [10]", storage.deleted)
	}
}

type orphanStorage struct {
	messages []telegramstore.DocumentMessage
	deleted  []int64
}

func (s *orphanStorage) ListDocumentMessages(context.Context, telegramstore.ListDocumentMessagesRequest) (telegramstore.DocumentMessagePage, error) {
	return telegramstore.DocumentMessagePage{Messages: s.messages, Exhausted: true}, nil
}
func (*orphanStorage) Upload(context.Context, telegramstore.UploadRequest) (telegramstore.StoredPart, error) {
	return telegramstore.StoredPart{}, errors.New("not used")
}
func (*orphanStorage) OpenRange(context.Context, telegramstore.RangeRequest) (io.ReadCloser, error) {
	return nil, errors.New("not used")
}
func (s *orphanStorage) DeleteMessages(_ context.Context, _, _ int64, ids []int64) error {
	s.deleted = append(s.deleted, ids...)
	return nil
}
func (*orphanStorage) CopyPart(context.Context, int64, int64, int64, int64) (telegramstore.StoredPart, error) {
	return telegramstore.StoredPart{}, errors.New("not used")
}
func (*orphanStorage) CreateChannel(context.Context, int64, string) (telegramstore.Channel, error) {
	return telegramstore.Channel{}, errors.New("not used")
}
func (*orphanStorage) DeleteChannel(context.Context, int64, int64) error {
	return errors.New("not used")
}
