//go:build integration

package jobs

import (
	"context"
	"testing"
	"time"

	"github.com/riverqueue/river"

	testpostgres "github.com/tgdrive/teldrive/v2/internal/testutil/postgres"
)

func TestEventCleanupWorkerDeletesOnlyExpiredEvents(t *testing.T) {
	db := testpostgres.New(t)
	ctx := context.Background()
	now := time.Date(2026, time.September, 23, 12, 0, 0, 0, time.UTC)
	if _, err := db.Pool.Exec(ctx, `INSERT INTO users (user_id) VALUES (1001)`); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if _, err := db.Pool.Exec(ctx, `
INSERT INTO user_events (user_id,event_type,resource_type,occurred_at) VALUES
    (1001,'old','test',$1),
    (1001,'cutoff','test',$2),
    (1001,'recent','test',$3)`, now.Add(-49*time.Hour), now.Add(-48*time.Hour), now.Add(-time.Hour)); err != nil {
		t.Fatalf("seed user events: %v", err)
	}

	worker := NewEventCleanupWorker(db.Pool)
	worker.now = func() time.Time { return now }
	if err := worker.Work(ctx, &river.Job[EventCleanupArgs]{Args: EventCleanupArgs{Retention: "48h"}}); err != nil {
		t.Fatalf("Work() error = %v", err)
	}

	var remaining, lastEventID, newestEventID int64
	if err := db.Pool.QueryRow(ctx, `SELECT
    (SELECT count(*) FROM user_events),
    (SELECT last_event_id FROM user_event_stream_state WHERE user_id=1001),
    (SELECT max(id) FROM user_events)`).Scan(&remaining, &lastEventID, &newestEventID); err != nil {
		t.Fatalf("inspect cleaned events: %v", err)
	}
	if remaining != 2 {
		t.Fatalf("remaining events = %d, want 2", remaining)
	}
	if lastEventID != newestEventID {
		t.Fatalf("stream state = %d, newest event = %d", lastEventID, newestEventID)
	}
}

func TestEventCleanupWorkerRejectsInvalidRetention(t *testing.T) {
	db := testpostgres.New(t)
	worker := NewEventCleanupWorker(db.Pool)
	for _, retention := range []string{"", "invalid", "0s", "-1h"} {
		err := worker.Work(context.Background(), &river.Job[EventCleanupArgs]{Args: EventCleanupArgs{Retention: retention}})
		if err == nil {
			t.Fatalf("Work() retention %q error = nil", retention)
		}
	}
}
