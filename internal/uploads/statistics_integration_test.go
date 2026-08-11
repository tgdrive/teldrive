//go:build integration

package uploads_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	testpostgres "github.com/tgdrive/teldrive/v2/internal/testutil/postgres"
	"github.com/tgdrive/teldrive/v2/internal/uploads"
)

func TestStatisticsReturnsDenseDailySeries(t *testing.T) {
	db := testpostgres.New(t)
	ctx := context.Background()
	if _, err := db.Pool.Exec(ctx, "INSERT INTO users (user_id) VALUES (1001)"); err != nil {
		t.Fatal(err)
	}
	todayFile := uuid.New()
	yesterdayFile := uuid.New()
	if _, err := db.Pool.Exec(ctx, `
INSERT INTO files (id, user_id, name, normalized_name, kind, mime_type, size, status, mod_time)
VALUES
    ($1, 1001, 'today.bin', 'today.bin', 'file', 'application/octet-stream', 7, 'active', now()),
    ($2, 1001, 'yesterday.bin', 'yesterday.bin', 'file', 'application/octet-stream', 5, 'active', now())
`, todayFile, yesterdayFile); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Pool.Exec(ctx, `
INSERT INTO upload_sessions (
    id, user_id, name, normalized_name, expected_size, part_size, state,
    mod_time, file_id, expires_at, completed_at
) VALUES
    ($1, 1001, 'today.bin', 'today.bin', 7, 1, 'completed', now(), $4, now() + interval '1 day', now()),
    ($2, 1001, 'yesterday.bin', 'yesterday.bin', 5, 1, 'completed', now(), $5, now() + interval '1 day', now() - interval '1 day'),
    ($3, 1001, 'open.bin', 'open.bin', 99, 1, 'open', now(), NULL, now() + interval '1 day', NULL)
`, uuid.New(), uuid.New(), uuid.New(), todayFile, yesterdayFile); err != nil {
		t.Fatal(err)
	}

	svc := uploads.NewService(db.Pool)
	items, err := svc.Statistics(ctx, 1001, 3)
	if err != nil {
		t.Fatalf("Statistics() error = %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("Statistics() returned %d days, want 3", len(items))
	}
	if !items[0].Date.Before(items[1].Date) || !items[1].Date.Before(items[2].Date) {
		t.Fatalf("days are not ascending: %#v", items)
	}
	if items[1].UploadedBytes != 5 || items[1].CompletedFiles != 1 {
		t.Fatalf("yesterday statistics = %#v", items[1])
	}
	if items[2].UploadedBytes != 7 || items[2].CompletedFiles != 1 {
		t.Fatalf("today statistics = %#v", items[2])
	}
	if items[0].UploadedBytes != 0 || items[0].CompletedFiles != 0 {
		t.Fatalf("empty day statistics = %#v", items[0])
	}
	if items[2].Date.Location() != time.UTC && items[2].Date.Location().String() != "UTC" {
		t.Fatalf("unexpected date location %v", items[2].Date.Location())
	}

	for _, input := range []struct {
		userID int64
		days   int32
	}{{0, 1}, {1001, 0}, {1001, 367}} {
		if _, err := svc.Statistics(ctx, input.userID, input.days); !errors.Is(err, uploads.ErrInvalidInput) {
			t.Fatalf("Statistics(%d, %d) error = %v", input.userID, input.days, err)
		}
	}
}
