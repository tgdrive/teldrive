package jobs

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/tgdrive/teldrive/v2/internal/db/sqlcgen"
	"github.com/tgdrive/teldrive/v2/internal/dbtypes"
)

const (
	PurgeSweepKind = "teldrive_purge_pending_files"
	PurgeQueue     = CleanupQueue
)

var ErrPurgeNotConfigured = errors.New("pending-file purge worker is not configured")

type PurgeService interface {
	Purge(context.Context, int64, uuid.UUID) error
}

type PurgeSweepArgs struct {
	BatchSize int32 `json:"batch_size,omitempty"`
}

func (PurgeSweepArgs) Kind() string { return PurgeSweepKind }

func (PurgeSweepArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: PurgeQueue, MaxAttempts: 3, Priority: 1}
}

type PendingFilePurgeWorker struct {
	river.WorkerDefaults[PurgeSweepArgs]
	pool    *pgxpool.Pool
	queries *sqlcgen.Queries
	service PurgeService
}

func NewPendingFilePurgeWorker(pool *pgxpool.Pool, service PurgeService) *PendingFilePurgeWorker {
	return &PendingFilePurgeWorker{pool: pool, queries: sqlcgen.New(pool), service: service}
}

func (w *PendingFilePurgeWorker) Timeout(*river.Job[PurgeSweepArgs]) time.Duration {
	return 30 * time.Minute
}

func (w *PendingFilePurgeWorker) Work(ctx context.Context, job *river.Job[PurgeSweepArgs]) error {
	if w == nil || w.pool == nil || w.service == nil {
		return ErrPurgeNotConfigured
	}
	batchSize := job.Args.BatchSize
	if batchSize <= 0 {
		batchSize = defaultBatchSize
	}
	rows, err := w.queries.ListDeletionPendingRoots(ctx, batchSize)
	if err != nil {
		return fmt.Errorf("list deletion-pending roots: %w", err)
	}
	for _, item := range rows {
		fileID, ok := dbtypes.GoogleUUID(item.FileID)
		if !ok {
			return fmt.Errorf("list deletion-pending roots: invalid file ID")
		}
		if err := w.purgeOne(ctx, item.UserID, fileID); err != nil {
			return err
		}
	}
	return nil
}

func (w *PendingFilePurgeWorker) purgeOne(ctx context.Context, userID int64, fileID uuid.UUID) error {
	if err := w.service.Purge(ctx, userID, fileID); err != nil {
		return fmt.Errorf("retry deletion-pending file %s for user %d: %w", fileID, userID, err)
	}
	return nil
}
