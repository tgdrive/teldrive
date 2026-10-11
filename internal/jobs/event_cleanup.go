package jobs

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/tgdrive/teldrive/v2/internal/db/sqlcgen"
)

const EventCleanupKind = "teldrive_cleanup_user_events"

var ErrEventCleanupNotConfigured = errors.New("event cleanup worker is not configured")

type EventCleanupArgs struct {
	Retention string `json:"retention"`
}

func (EventCleanupArgs) Kind() string { return EventCleanupKind }

func (EventCleanupArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: CleanupQueue, MaxAttempts: 3, Priority: 2}
}

type EventCleanupWorker struct {
	river.WorkerDefaults[EventCleanupArgs]
	queries *sqlcgen.Queries
	now     func() time.Time
}

func NewEventCleanupWorker(pool *pgxpool.Pool) *EventCleanupWorker {
	return &EventCleanupWorker{queries: sqlcgen.New(pool), now: time.Now}
}

func (w *EventCleanupWorker) Timeout(*river.Job[EventCleanupArgs]) time.Duration {
	return 30 * time.Minute
}

func (w *EventCleanupWorker) Work(ctx context.Context, job *river.Job[EventCleanupArgs]) error {
	if w == nil || w.queries == nil || w.now == nil {
		return ErrEventCleanupNotConfigured
	}
	retention, err := time.ParseDuration(job.Args.Retention)
	if err != nil || retention <= 0 {
		return fmt.Errorf("invalid event retention %q", job.Args.Retention)
	}
	cutoff := w.now().UTC().Add(-retention)
	deleted, err := w.queries.DeleteUserEventsBefore(ctx, pgtype.Timestamptz{Time: cutoff, Valid: true})
	if err != nil {
		return fmt.Errorf("delete expired user events: %w", err)
	}
	if job.JobRow != nil {
		return river.RecordOutput(ctx, struct {
			Deleted int64     `json:"deleted"`
			Cutoff  time.Time `json:"cutoff"`
		}{Deleted: deleted, Cutoff: cutoff})
	}
	return nil
}
