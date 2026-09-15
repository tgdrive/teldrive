package jobs

import (
	"context"
	"errors"
	"fmt"
	"sort"
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
	PurgeMany(context.Context, int64, []uuid.UUID) error
}

type PurgeSweepArgs struct{}

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
	return 2 * time.Hour
}

func (w *PendingFilePurgeWorker) Work(ctx context.Context, job *river.Job[PurgeSweepArgs]) error {
	if w == nil || w.pool == nil || w.service == nil {
		return ErrPurgeNotConfigured
	}
	for {
		rows, err := w.queries.ListDeletionPendingRoots(ctx)
		if err != nil {
			return fmt.Errorf("list deletion-pending roots: %w", err)
		}
		if len(rows) == 0 {
			return nil
		}
		byUser := make(map[int64][]uuid.UUID)
		for _, item := range rows {
			fileID, ok := dbtypes.GoogleUUID(item.FileID)
			if !ok {
				return fmt.Errorf("list deletion-pending roots: invalid file ID")
			}
			byUser[item.UserID] = append(byUser[item.UserID], fileID)
		}
		userIDs := make([]int64, 0, len(byUser))
		for userID := range byUser {
			userIDs = append(userIDs, userID)
		}
		sort.Slice(userIDs, func(i, j int) bool { return userIDs[i] < userIDs[j] })
		for _, userID := range userIDs {
			if err := w.service.PurgeMany(ctx, userID, byUser[userID]); err != nil {
				return fmt.Errorf("retry deletion-pending files for user %d: %w", userID, err)
			}
		}
	}
}
