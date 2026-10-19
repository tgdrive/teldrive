//go:build integration

package database_test

import (
	"context"
	"io/fs"
	"strings"
	"testing"

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

func TestReconcileOriginalMigrationNumbersPreservesAccountData(t *testing.T) {
	db := testpostgres.New(t)
	ctx := context.Background()
	// Model the original 2.0.7 schema: upstream tables exist under versions
	// 8-12, while edition-only bot names and incoming Spam do not exist yet.
	_, err := db.Pool.Exec(ctx, `
INSERT INTO users(user_id) VALUES(99);
INSERT INTO files(id,user_id,name,kind,size,status,mod_time)
VALUES('123e4567-e89b-12d3-a456-426614174000',99,'Keep me','folder',0,'active',now());
INSERT INTO file_bookmarks(user_id,file_id) VALUES(99,'123e4567-e89b-12d3-a456-426614174000');
INSERT INTO user_preferences(user_id,theme,locale,upload_concurrency) VALUES(99,'dark','es',7);
DROP TABLE incoming_spam;
ALTER TABLE bots DROP COLUMN name;
DELETE FROM migrations WHERE version_id > 12;`)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(ctx, database.Config{URL: db.URL}); err != nil {
		t.Fatalf("upgrade original schema: %v", err)
	}
	if _, err := db.Pool.Exec(ctx, "SELECT name FROM bots; SELECT grantee_id FROM incoming_spam"); err != nil {
		t.Fatalf("edition objects were not restored: %v", err)
	}
	var theme, locale string
	var concurrency, bookmarks int
	if err := db.Pool.QueryRow(ctx, "SELECT theme,locale,upload_concurrency FROM user_preferences WHERE user_id=99").Scan(&theme, &locale, &concurrency); err != nil {
		t.Fatal(err)
	}
	if err := db.Pool.QueryRow(ctx, "SELECT count(*) FROM file_bookmarks WHERE user_id=99").Scan(&bookmarks); err != nil {
		t.Fatal(err)
	}
	if theme != "dark" || locale != "es" || concurrency != 7 || bookmarks != 1 {
		t.Fatalf("account data changed: %s %s %d %d", theme, locale, concurrency, bookmarks)
	}
}
