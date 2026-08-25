//go:build integration

package database_test

import (
	"context"
	"strings"
	"testing"

	"github.com/tgdrive/teldrive/v2/internal/database"
	testpostgres "github.com/tgdrive/teldrive/v2/internal/testutil/postgres"
)

func TestMigrateAndOpenAgainstPostgres18(t *testing.T) {
	db := testpostgres.New(t)
	ctx := context.Background()

	if err := database.Migrate(ctx, database.Config{URL: db.URL, Schema: "teldrive"}); err != nil {
		t.Fatalf("second migration run should be idempotent: %v", err)
	}

	var version string
	if err := db.Pool.QueryRow(ctx, "SHOW server_version").Scan(&version); err != nil {
		t.Fatalf("read PostgreSQL version: %v", err)
	}
	if !strings.HasPrefix(version, "18.") {
		t.Fatalf("expected PostgreSQL 18, got %q", version)
	}

	var appTableCount int
	if err := db.Pool.QueryRow(ctx, `
SELECT count(*)
FROM information_schema.tables
WHERE table_schema = 'teldrive'
  AND table_type = 'BASE TABLE'
  AND table_name IN (
    'users', 'sessions', 'api_keys', 'bots', 'channels',
    'files', 'file_parts', 'upload_sessions', 'upload_parts',
    'file_shares', 'idempotency_keys', 'audit_events',
    'user_events', 'user_event_stream_state', 'event_stream_tickets'
  )`).Scan(&appTableCount); err != nil {
		t.Fatalf("count migrated application tables: %v", err)
	}
	if appTableCount != 15 {
		t.Fatalf("migrated application table count = %d, want 15", appTableCount)
	}

	var publicTableCount int
	if err := db.Pool.QueryRow(ctx, `
SELECT count(*)
FROM information_schema.tables
WHERE table_schema = 'public'
  AND table_type = 'BASE TABLE'`).Scan(&publicTableCount); err != nil {
		t.Fatalf("count public base tables: %v", err)
	}
	if publicTableCount != 0 {
		t.Fatalf("public base table count = %d, want 0", publicTableCount)
	}

	var migrationVersion int64
	if err := db.Pool.QueryRow(ctx, "SELECT max(version_id) FROM teldrive.migrations WHERE is_applied").Scan(&migrationVersion); err != nil {
		t.Fatalf("read migration version: %v", err)
	}
	if migrationVersion != 6 {
		t.Fatalf("migration version = %d, want 6", migrationVersion)
	}
}

func TestMigrateCustomSchema(t *testing.T) {
	db := testpostgres.New(t)
	ctx := context.Background()
	const schema = "teldrive_custom_test"

	if err := database.Migrate(ctx, database.Config{URL: db.URL, Schema: schema}); err != nil {
		t.Fatalf("migrate custom schema: %v", err)
	}

	for _, table := range []string{"users", "migrations", "river_job", "river_migration"} {
		var exists bool
		if err := db.Pool.QueryRow(ctx, `
SELECT EXISTS (
    SELECT 1
    FROM information_schema.tables
    WHERE table_schema = $1 AND table_name = $2 AND table_type = 'BASE TABLE'
)`, schema, table).Scan(&exists); err != nil {
			t.Fatalf("inspect %s.%s: %v", schema, table, err)
		}
		if !exists {
			t.Fatalf("expected table %s.%s", schema, table)
		}
	}
}
