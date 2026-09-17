package jobs

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/tgdrive/teldrive/v2/internal/db/sqlcgen"
	"github.com/tgdrive/teldrive/v2/internal/dbtypes"
)

const TrashCleanupSweepKind = "teldrive_cleanup_trash"

var ErrTrashCleanupNotConfigured = errors.New("trash cleanup worker is not configured")

type TrashCleanupSweepArgs struct {
	Retention string `json:"retention,omitempty"`
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
	return 2 * time.Hour
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
	deletedBefore := dbtypes.Time(w.now().Add(-retention))
	for {
		rows, err := w.queries.ListTrashedRootsBefore(ctx, deletedBefore)
		if err != nil {
			return fmt.Errorf("list expired trash roots: %w", err)
		}
		if len(rows) == 0 {
			return nil
		}
		byUser := make(map[int64][]uuid.UUID)
		for _, item := range rows {
			fileID, ok := dbtypes.GoogleUUID(item.FileID)
			if !ok {
				return errors.New("expired trash root has invalid file ID")
			}
			byUser[item.UserID] = append(byUser[item.UserID], fileID)
		}
		userIDs := slices.Sorted(maps.Keys(byUser))
		for _, userID := range userIDs {
			if err := w.service.PurgeMany(ctx, userID, byUser[userID]); err != nil {
				return fmt.Errorf("purge expired trash files for user %d: %w", userID, err)
			}
		}
	}
}
