//go:build integration

package database_test

import (
	"context"
	"io/fs"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/pressly/goose/v3"
	"github.com/tgdrive/teldrive/v2/db/migrations"
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
	if want := latestEmbeddedMigrationVersion(t); migrationVersion != want {
		t.Fatalf("migration version = %d, want %d", migrationVersion, want)
	}

	var normalizedNameColumns int
	if err := db.Pool.QueryRow(ctx, `
SELECT count(*)
FROM information_schema.columns
WHERE table_schema = 'teldrive'
  AND table_name IN ('files', 'upload_sessions')
  AND column_name = 'normalized_name'`).Scan(&normalizedNameColumns); err != nil {
		t.Fatalf("count normalized name columns: %v", err)
	}
	if normalizedNameColumns != 0 {
		t.Fatalf("normalized name columns = %d, want 0", normalizedNameColumns)
	}
}

func latestEmbeddedMigrationVersion(t *testing.T) int64 {
	t.Helper()
	names, err := fs.Glob(migrations.Files, "*.sql")
	if err != nil {
		t.Fatalf("list embedded migrations: %v", err)
	}
	var latest int64
	for _, name := range names {
		version, err := goose.NumericComponent(name)
		if err != nil {
			t.Fatalf("parse migration version from %s: %v", name, err)
		}
		latest = max(latest, version)
	}
	if latest == 0 {
		t.Fatal("no embedded migrations found")
	}
	return latest
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

func TestRollbackOneMigrationAndReapply(t *testing.T) {
	for _, schema := range []string{database.DefaultSchema, "tenant_rollback"} {
		t.Run(schema, func(t *testing.T) {
			db := testpostgres.New(t)
			ctx := context.Background()
			cfg := database.Config{URL: db.URL, Schema: schema}
			if err := database.Migrate(ctx, cfg); err != nil {
				t.Fatal(err)
			}
			table := pgx.Identifier{schema, "migrations"}.Sanitize()
			var latest, previous int64
			if err := db.Pool.QueryRow(ctx, "SELECT max(version_id) FROM "+table+" WHERE is_applied").Scan(&latest); err != nil {
				t.Fatal(err)
			}
			if err := db.Pool.QueryRow(ctx, "SELECT max(version_id) FROM "+table+" WHERE is_applied AND version_id < $1", latest).Scan(&previous); err != nil {
				t.Fatal(err)
			}
			var riverBefore int
			if err := db.Pool.QueryRow(ctx, "SELECT count(*) FROM "+pgx.Identifier{schema, "river_migration"}.Sanitize()).Scan(&riverBefore); err != nil {
				t.Fatal(err)
			}
			if err := database.Rollback(ctx, cfg); err != nil {
				t.Fatal(err)
			}
			var after int64
			if err := db.Pool.QueryRow(ctx, "SELECT max(version_id) FROM "+table+" WHERE is_applied").Scan(&after); err != nil {
				t.Fatal(err)
			}
			if after != previous {
				t.Fatalf("rollback version = %d, want exactly previous %d (was %d)", after, previous, latest)
			}
			var riverAfter int
			if err := db.Pool.QueryRow(ctx, "SELECT count(*) FROM "+pgx.Identifier{schema, "river_migration"}.Sanitize()).Scan(&riverAfter); err != nil {
				t.Fatal(err)
			}
			if riverAfter != riverBefore {
				t.Fatalf("rollback changed River migrations: %d -> %d", riverBefore, riverAfter)
			}
			if schema != database.DefaultSchema {
				var other int64
				if err := db.Pool.QueryRow(ctx, "SELECT max(version_id) FROM teldrive.migrations WHERE is_applied").Scan(&other); err != nil {
					t.Fatal(err)
				}
				if other != latest {
					t.Fatalf("rollback affected another schema: %d", other)
				}
			}
			if err := database.Migrate(ctx, cfg); err != nil {
				t.Fatal(err)
			}
			if err := db.Pool.QueryRow(ctx, "SELECT max(version_id) FROM "+table+" WHERE is_applied").Scan(&after); err != nil {
				t.Fatal(err)
			}
			if after != latest {
				t.Fatalf("up version = %d, want %d", after, latest)
			}
		})
	}
}

func TestRollbackMissingSchemaDoesNotCreateObjects(t *testing.T) {
	db := testpostgres.New(t)
	ctx := context.Background()
	err := database.Rollback(ctx, database.Config{URL: db.URL, Schema: "not_migrated"})
	if err == nil || !strings.Contains(err.Error(), "no application migrations") {
		t.Fatalf("missing-schema rollback = %v", err)
	}
	var exists bool
	if err := db.Pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM pg_namespace WHERE nspname = 'not_migrated')").Scan(&exists); err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Fatal("rollback created an unexpected schema")
	}
}
