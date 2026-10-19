package jobs

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/divyam234/riverpro"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/tgdrive/teldrive/v2/internal/db/sqlcgen"
)

const FolderSizeRollupKind = "teldrive_rollup_folder_sizes"

var ErrFolderSizeRollupNotConfigured = errors.New("folder size rollup worker is not configured")

// FolderSizeRollupArgs recomputes cumulative folder sizes for the whole drive
// in a single statement.
type FolderSizeRollupArgs struct{}

func (FolderSizeRollupArgs) Kind() string { return FolderSizeRollupKind }

func (FolderSizeRollupArgs) EphemeralOpts() riverpro.EphemeralOpts {
	return riverpro.EphemeralOpts{}
}

var _ riverpro.JobArgsWithEphemeralOpts = FolderSizeRollupArgs{}

func (FolderSizeRollupArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: CleanupQueue, MaxAttempts: 1, Priority: 3}
}

type FolderSizeRollupOutput struct {
	Updated     int       `json:"updated"`
	CompletedAt time.Time `json:"completedAt"`
}

type FolderSizeRollupWorker struct {
	river.WorkerDefaults[FolderSizeRollupArgs]
	pool    *pgxpool.Pool
	queries *sqlcgen.Queries
	now     func() time.Time
}

func NewFolderSizeRollupWorker(pool *pgxpool.Pool) *FolderSizeRollupWorker {
	return &FolderSizeRollupWorker{pool: pool, queries: sqlcgen.New(pool), now: time.Now}
}

func (w *FolderSizeRollupWorker) Timeout(*river.Job[FolderSizeRollupArgs]) time.Duration {
	return 2 * time.Hour
}

func (w *FolderSizeRollupWorker) Work(ctx context.Context, job *river.Job[FolderSizeRollupArgs]) error {
	if w == nil || w.pool == nil || w.queries == nil {
		return ErrFolderSizeRollupNotConfigured
	}
	updated, err := w.queries.RollupFolderSizes(ctx)
	if err != nil {
		return fmt.Errorf("roll up folder sizes: %w", err)
	}
	if job.JobRow != nil {
		return river.RecordOutput(ctx, FolderSizeRollupOutput{
			Updated: int(updated), CompletedAt: w.now().UTC(),
		})
	}
	return nil
}
