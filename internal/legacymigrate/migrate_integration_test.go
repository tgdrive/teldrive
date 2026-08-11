//go:build integration

package legacymigrate_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/tgdrive/teldrive/v2/internal/database"
	"github.com/tgdrive/teldrive/v2/internal/legacymigrate"
	testpostgres "github.com/tgdrive/teldrive/v2/internal/testutil/postgres"
)

func TestRunCopiesLegacyDatabase(t *testing.T) {
	ctx := context.Background()
	source := testpostgres.New(t)

	if _, err := source.Pool.Exec(ctx, `
DROP SCHEMA teldrive CASCADE;
CREATE SCHEMA teldrive;
CREATE TABLE IF NOT EXISTS public.goose_db_version(id integer);
CREATE TABLE teldrive.users (
    user_id bigint PRIMARY KEY, name text, user_name text NOT NULL,
    is_premium boolean NOT NULL, created_at timestamptz NOT NULL, updated_at timestamptz NOT NULL
);
CREATE TABLE teldrive.channels (
    channel_id bigint PRIMARY KEY, channel_name text NOT NULL, user_id bigint NOT NULL,
    selected boolean, created_at timestamptz NOT NULL
);
CREATE TABLE teldrive.bots (
    user_id bigint NOT NULL, token text NOT NULL, bot_id bigint NOT NULL,
    PRIMARY KEY(user_id, token)
);
CREATE TABLE teldrive.files (
    id uuid PRIMARY KEY, name text NOT NULL, type text NOT NULL, mime_type text NOT NULL,
    size bigint, user_id bigint NOT NULL, parent_id uuid, status text,
    channel_id bigint, parts jsonb, encrypted boolean NOT NULL DEFAULT false,
    hash text, created_at timestamptz NOT NULL, updated_at timestamptz NOT NULL
);
INSERT INTO teldrive.users VALUES (101, 'User', 'user', true, now(), now());
INSERT INTO teldrive.channels VALUES (201, 'Channel', 101, true, now());
INSERT INTO teldrive.bots VALUES (101, '123:token', 301);
`); err != nil {
		t.Fatalf("seed legacy schema: %v", err)
	}
	folderID := uuid.New()
	fileID := uuid.New()
	if _, err := source.Pool.Exec(ctx, `
INSERT INTO teldrive.files(id,name,type,mime_type,size,user_id,parent_id,status,channel_id,parts,encrypted,created_at,updated_at)
VALUES
($1,'Folder','folder','application/octet-stream',NULL,101,NULL,'active',NULL,'[]',false,now(),now()),
($2,'File.bin','file','application/octet-stream',10,101,$1,'active',201,'[{"id":401}]',false,now(),now())`, folderID, fileID); err != nil {
		t.Fatalf("seed legacy files: %v", err)
	}

	report, err := legacymigrate.Run(ctx, legacymigrate.Config{
		SourceURL:            source.URL,
		Target:               database.Config{URL: source.URL, Schema: "teldrive_v2_staging_test"},
		LegacySchema:         "teldrive",
		FinalSchema:          "teldrive",
		BackupSchema:         "teldrive_legacy_backup_test",
		DataKey:              "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		EncryptionKeyVersion: 1,
		Apply:                true,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if report.Users != 1 || report.Channels != 1 || report.Bots != 1 || report.Folders != 1 || report.Files != 1 || report.FileParts != 1 {
		t.Fatalf("unexpected report: %+v", report)
	}

	var users, channels, bots, files, parts int
	if err := source.Pool.QueryRow(ctx, `SELECT
(SELECT count(*) FROM teldrive.users),
(SELECT count(*) FROM teldrive.channels),
(SELECT count(*) FROM teldrive.bots),
(SELECT count(*) FROM teldrive.files),
(SELECT count(*) FROM teldrive.file_parts)`).Scan(&users, &channels, &bots, &files, &parts); err != nil {
		t.Fatalf("count target rows: %v", err)
	}
	if users != 1 || channels != 1 || bots != 1 || files != 2 || parts != 1 {
		t.Fatalf("target counts = %d,%d,%d,%d,%d", users, channels, bots, files, parts)
	}
	var unresolved bool
	if err := source.Pool.QueryRow(ctx, `SELECT plain_size IS NULL AND stored_size IS NULL FROM teldrive.file_parts WHERE file_id=$1`, fileID).Scan(&unresolved); err != nil {
		t.Fatalf("inspect migrated part: %v", err)
	}
	if !unresolved {
		t.Fatal("legacy part sizes were unexpectedly populated")
	}
	var backupUserCount int
	if err := source.Pool.QueryRow(ctx, `SELECT count(*) FROM teldrive_legacy_backup_test.users`).Scan(&backupUserCount); err != nil {
		t.Fatalf("inspect backup schema: %v", err)
	}
	if backupUserCount != 1 {
		t.Fatalf("backup user count = %d, want 1", backupUserCount)
	}
	var gooseMoved bool
	if err := source.Pool.QueryRow(ctx, `SELECT to_regclass('teldrive_legacy_backup_test.goose_db_version') IS NOT NULL AND to_regclass('public.goose_db_version') IS NULL`).Scan(&gooseMoved); err != nil {
		t.Fatalf("inspect moved goose table: %v", err)
	}
	if !gooseMoved {
		t.Fatal("legacy goose table was not moved into the backup schema")
	}
}
