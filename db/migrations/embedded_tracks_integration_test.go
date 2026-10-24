//go:build integration

package migrations_test

import (
	"context"
	"strings"
	"testing"

	"github.com/tgdrive/teldrive/v2/db/migrations"
	testpostgres "github.com/tgdrive/teldrive/v2/internal/testutil/postgres"
)

// Migration 13 adds the embedded-tracks preferences. It must be safe to run again: a database that
// already has the columns (and values in them) has to come through unchanged, not fail.
func TestEmbeddedTracksMigrationCanRunAgain(t *testing.T) {
	db := testpostgres.New(t) // already applied every migration, this one included
	ctx := context.Background()

	data, err := migrations.Files.ReadFile("000013_embedded_tracks.sql")
	if err != nil {
		t.Fatal(err)
	}
	up := strings.Split(strings.Split(string(data), "-- +goose Up")[1], "-- +goose Down")[0]
	up = strings.ReplaceAll(up, "/* TEMPLATE: schema */", "")
	// The current preferences table is JSONB-only. Recreate the historical
	// columns in this isolated test before exercising migration 13's idempotency.
	if _, err := db.Pool.Exec(ctx, up); err != nil {
		t.Fatal(err)
	}

	if _, err := db.Pool.Exec(ctx, "INSERT INTO users (user_id) VALUES (1), (2), (3)"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Pool.Exec(ctx, `
INSERT INTO user_preferences (user_id, embedded_tracks, embedded_tracks_on_shares)
VALUES (1, true, true), (2, true, false), (3, false, false)`); err != nil {
		t.Fatal(err)
	}

	// Twice, in case the first run is the one that happens to work.
	for run := 1; run <= 2; run++ {
		if _, err := db.Pool.Exec(ctx, up); err != nil {
			t.Fatalf("running the migration again (run %d): %v", run, err)
		}
	}

	rows, err := db.Pool.Query(ctx, "SELECT user_id, embedded_tracks, embedded_tracks_on_shares FROM user_preferences ORDER BY user_id")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	type values struct{ tracks, shares bool }
	got := map[int64]values{}
	for rows.Next() {
		var id int64
		var v values
		if err := rows.Scan(&id, &v.tracks, &v.shares); err != nil {
			t.Fatal(err)
		}
		got[id] = v
	}
	if want := map[int64]values{1: {true, true}, 2: {true, false}, 3: {false, false}}; len(got) != 3 || got[1] != want[1] || got[2] != want[2] || got[3] != want[3] {
		t.Fatalf("values changed by running the migration again: %+v", got)
	}

	// The rule is still enforced afterwards: the shares option cannot be on without the first.
	if _, err := db.Pool.Exec(ctx, "UPDATE user_preferences SET embedded_tracks = false WHERE user_id = 1"); err == nil {
		t.Fatal("the constraint is gone after running the migration again")
	}
	var constraints int
	if err := db.Pool.QueryRow(ctx, `SELECT count(*) FROM pg_constraint WHERE conname = 'user_preferences_embedded_tracks_on_shares_needs_tracks'`).Scan(&constraints); err != nil || constraints != 1 {
		t.Fatalf("constraints named like that = %d, %v; want exactly 1", constraints, err)
	}
}
