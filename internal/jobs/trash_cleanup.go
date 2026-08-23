package jobs

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/tgdrive/teldrive/v2/internal/db/sqlcgen"
	"github.com/tgdrive/teldrive/v2/internal/dbtypes"
)

const TrashCleanupSweepKind = "teldrive_cleanup_trash"

var ErrTrashCleanupNotConfigured = errors.New("trash cleanup worker is not configured")

type TrashCleanupSweepArgs struct {
	Retention string `json:"retention,omitempty"`
	BatchSize int32  `json:"batch_size,omitempty"`
}

func (TrashCleanupSweepArgs) Kind() string { return TrashCleanupSweepKind }

func (TrashCleanupSweepArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: CleanupQueue, MaxAttempts: 3, Priority: 1}
}

type TrashCleanupWorker struct {
	river.WorkerDefaults[TrashCleanupSweepArgs]
	pool    *pgxpool.Pool
	queries *sqlcgen.Queries
	service PurgeService
	now     func() time.Time
}

func NewTrashCleanupWorker(pool *pgxpool.Pool, service PurgeService) *TrashCleanupWorker {
	return &TrashCleanupWorker{pool: pool, queries: sqlcgen.New(pool), service: service, now: time.Now}
}

func (w *TrashCleanupWorker) Timeout(*river.Job[TrashCleanupSweepArgs]) time.Duration {
	return 30 * time.Minute
}

func (w *TrashCleanupWorker) Work(ctx context.Context, job *river.Job[TrashCleanupSweepArgs]) error {
	if w == nil || w.pool == nil || w.service == nil {
		return ErrTrashCleanupNotConfigured
	}
	retentionText := job.Args.Retention
	if retentionText == "" {
		retentionText = "720h"
	}
	retention, err := time.ParseDuration(retentionText)
	if err != nil || retention <= 0 {
		return fmt.Errorf("invalid trash retention %q", retentionText)
	}
	batchSize := job.Args.BatchSize
	if batchSize <= 0 {
		batchSize = defaultBatchSize
	}
	rows, err := w.queries.ListTrashedRootsBefore(ctx, sqlcgen.ListTrashedRootsBeforeParams{
		DeletedBefore: dbtypes.Time(w.now().Add(-retention)),
		BatchSize:     batchSize,
	})
	if err != nil {
		return fmt.Errorf("list expired trash roots: %w", err)
	}
	for _, item := range rows {
		fileID, ok := dbtypes.GoogleUUID(item.FileID)
		if !ok {
			return errors.New("expired trash root has invalid file ID")
		}
		if err := w.service.Purge(ctx, item.UserID, fileID); err != nil {
			return fmt.Errorf("purge expired trash file %s for user %d: %w", fileID, item.UserID, err)
		}
	}
	return nil
}
