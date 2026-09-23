//go:build integration

package legacymigrate_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/tgdrive/teldrive/v2/internal/bots"
	"github.com/tgdrive/teldrive/v2/internal/catalog"
	"github.com/tgdrive/teldrive/v2/internal/database"
	"github.com/tgdrive/teldrive/v2/internal/legacymigrate"
	"github.com/tgdrive/teldrive/v2/internal/secureblob"
	testpostgres "github.com/tgdrive/teldrive/v2/internal/testutil/postgres"
)

func TestMigrateIfNeededCopiesLegacyDatabase(t *testing.T) {
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
INSERT INTO teldrive.users VALUES
    (101, 'Owner', 'owner', true, now() - interval '1 hour', now() - interval '1 hour'),
    (102, 'User', 'user', false, now(), now());
INSERT INTO teldrive.channels VALUES (201, 'Channel', 101, true, now());
INSERT INTO teldrive.bots VALUES
    (101, '301:a-invalid', 301),
    (101, '301:b-valid', 301);
`); err != nil {
		t.Fatalf("seed legacy schema: %v", err)
	}
	syntheticRootID := uuid.New()
	folderID := uuid.New()
	fileID := uuid.New()
	rootFileID := uuid.New()
	if _, err := source.Pool.Exec(ctx, `
INSERT INTO teldrive.files(id,name,type,mime_type,size,user_id,parent_id,status,channel_id,parts,encrypted,created_at,updated_at)
VALUES
($1,'root','folder','drive/folder',NULL,101,NULL,'active',NULL,'[]',false,now(),now()),
($2,'Folder','folder','application/octet-stream',NULL,101,$1,'active',NULL,'[]',false,now(),now()),
($3,'File.bin','file','application/octet-stream',10,101,$2,'active',201,'[{"id":401}]',false,now(),now()),
($4,'Top-level empty.bin','file','application/octet-stream',0,101,$1,'active',NULL,'[]',false,now(),now())`, syntheticRootID, folderID, fileID, rootFileID); err != nil {
		t.Fatalf("seed legacy files: %v", err)
	}

	verifier := verifierFunc(func(_ context.Context, token string) (bots.Identity, error) {
		if token == "301:b-valid" {
			return bots.Identity{ID: 301, Username: "storage_bot"}, nil
		}
		return bots.Identity{}, bots.ErrNotBot
	})
	if _, _, err := legacymigrate.MigrateIfNeeded(ctx, database.Config{URL: source.URL}, "", verifier); err == nil || !strings.Contains(err.Error(), "security.data-key") {
		t.Fatalf("empty data-key error = %v", err)
	}
	report, migrated, err := legacymigrate.MigrateIfNeeded(
		ctx,
		database.Config{URL: source.URL},
		"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		verifier,
	)
	if err != nil {
		t.Fatalf("MigrateIfNeeded() error = %v", err)
	}
	if !migrated {
		t.Fatal("legacy database was not migrated")
	}
	if report.Users != 2 || report.Channels != 1 || report.Bots != 1 || report.Folders != 1 || report.Files != 2 || report.FileParts != 1 || report.SkippedZero != 1 {
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
	if users != 2 || channels != 1 || bots != 1 || files != 3 || parts != 1 {
		t.Fatalf("target counts = %d,%d,%d,%d,%d", users, channels, bots, files, parts)
	}
	var tokenCiphertext []byte
	if err := source.Pool.QueryRow(ctx, `SELECT token_ciphertext FROM teldrive.bots WHERE user_id=101 AND bot_id=301`).Scan(&tokenCiphertext); err != nil {
		t.Fatalf("load migrated bot token: %v", err)
	}
	cipher, err := secureblob.New("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")
	if err != nil {
		t.Fatalf("create data-key cipher: %v", err)
	}
	plainToken, err := cipher.Open("bot-token", tokenCiphertext)
	if err != nil {
		t.Fatalf("decrypt migrated bot token: %v", err)
	}
	if string(plainToken) != "301:b-valid" {
		t.Fatalf("migrated bot token = %q, want valid duplicate", plainToken)
	}

	var ownerRole, userRole string
	if err := source.Pool.QueryRow(ctx, `SELECT
(SELECT role::text FROM teldrive.users WHERE user_id=101),
(SELECT role::text FROM teldrive.users WHERE user_id=102)`).Scan(&ownerRole, &userRole); err != nil {
		t.Fatalf("inspect migrated roles: %v", err)
	}
	if ownerRole != "owner" || userRole != "user" {
		t.Fatalf("migrated roles = %q, %q; want owner, user", ownerRole, userRole)
	}
	var rootParentMissing bool
	if err := source.Pool.QueryRow(ctx, `SELECT parent_id IS NULL FROM teldrive.files WHERE id=$1`, rootFileID).Scan(&rootParentMissing); err != nil {
		t.Fatalf("inspect top-level file: %v", err)
	}
	if !rootParentMissing {
		t.Fatal("synthetic root child was not promoted to the v2 root")
	}
	var folderParentMissing, nestedFileParentPreserved, syntheticRootMissing bool
	if err := source.Pool.QueryRow(ctx, `SELECT
(SELECT parent_id IS NULL FROM teldrive.files WHERE id=$1),
(SELECT parent_id = $1 FROM teldrive.files WHERE id=$2),
NOT EXISTS (SELECT 1 FROM teldrive.files WHERE id=$3)`, folderID, fileID, syntheticRootID).Scan(&folderParentMissing, &nestedFileParentPreserved, &syntheticRootMissing); err != nil {
		t.Fatalf("inspect migrated root hierarchy: %v", err)
	}
	if !folderParentMissing || !nestedFileParentPreserved || !syntheticRootMissing {
		t.Fatalf("migrated root hierarchy = folder parent missing %v, nested parent preserved %v, synthetic root missing %v", folderParentMissing, nestedFileParentPreserved, syntheticRootMissing)
	}
	var unresolved bool
	if err := source.Pool.QueryRow(ctx, `SELECT plain_size IS NULL AND stored_size IS NULL FROM teldrive.file_parts WHERE file_id=$1`, fileID).Scan(&unresolved); err != nil {
		t.Fatalf("inspect migrated part: %v", err)
	}
	if !unresolved {
		t.Fatal("legacy part sizes were unexpectedly populated")
	}
	var backupUserCount, backupBotCount int
	if err := source.Pool.QueryRow(ctx, `SELECT
(SELECT count(*) FROM `+pgx.Identifier{report.BackupSchema}.Sanitize()+`.users),
(SELECT count(*) FROM `+pgx.Identifier{report.BackupSchema}.Sanitize()+`.bots)`).Scan(&backupUserCount, &backupBotCount); err != nil {
		t.Fatalf("inspect backup schema: %v", err)
	}
	if backupUserCount != 2 || backupBotCount != 2 {
		t.Fatalf("backup counts = users %d, bots %d; want 2, 2", backupUserCount, backupBotCount)
	}
	var gooseMoved bool
	if err := source.Pool.QueryRow(ctx, `SELECT to_regclass($1) IS NOT NULL AND to_regclass('public.goose_db_version') IS NULL`, report.BackupSchema+".goose_db_version").Scan(&gooseMoved); err != nil {
		t.Fatalf("inspect moved goose table: %v", err)
	}
	if !gooseMoved {
		t.Fatal("legacy goose table was not moved into the backup schema")
	}
	var eventCount, streamStateCount int
	if err := source.Pool.QueryRow(ctx, `SELECT
(SELECT count(*) FROM teldrive.user_events),
(SELECT count(*) FROM teldrive.user_event_stream_state)`).Scan(&eventCount, &streamStateCount); err != nil {
		t.Fatalf("inspect migrated event history: %v", err)
	}
	if eventCount != 0 || streamStateCount != 0 {
		t.Fatalf("migration-generated event history = events %d, stream states %d; want 0, 0", eventCount, streamStateCount)
	}

	service := catalog.NewService(source.Pool, nil)
	if _, err := service.Move(ctx, 101, rootFileID, &folderID, nil); err != nil {
		t.Fatalf("move file after schema cutover: %v", err)
	}
	if _, err := source.Pool.Exec(ctx, `UPDATE teldrive.channels SET health='healthy' WHERE user_id=101 AND channel_id=201`); err != nil {
		t.Fatalf("update channel after schema cutover: %v", err)
	}
	uploadID := uuid.New()
	if _, err := source.Pool.Exec(ctx, `
INSERT INTO teldrive.upload_sessions (
    id,user_id,parent_id,name,expected_size,mod_time,part_size,expires_at
) VALUES ($1,101,$2,'new.bin',1,now(),1,now()+interval '1 hour')`, uploadID, folderID); err != nil {
		t.Fatalf("create upload after schema cutover: %v", err)
	}
	if _, err := source.Pool.Exec(ctx, `
INSERT INTO teldrive.file_shares (file_id,owner_id,token_prefix,token_hash)
VALUES ($1,101,'prefix',decode('01020304','hex'))`, rootFileID); err != nil {
		t.Fatalf("create share after schema cutover: %v", err)
	}
	rows, err := source.Pool.Query(ctx, `SELECT event_type, count(*) FROM teldrive.user_events GROUP BY event_type`)
	if err != nil {
		t.Fatalf("inspect post-cutover events: %v", err)
	}
	events := make(map[string]int)
	for rows.Next() {
		var eventType string
		var count int
		if err := rows.Scan(&eventType, &count); err != nil {
			rows.Close()
			t.Fatalf("scan post-cutover event: %v", err)
		}
		events[eventType] = count
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate post-cutover events: %v", err)
	}
	wantEvents := map[string]int{
		"file.updated":    1,
		"channel.updated": 1,
		"upload.created":  1,
		"share.created":   1,
	}
	if len(events) != len(wantEvents) {
		t.Fatalf("post-cutover events = %#v, want %#v", events, wantEvents)
	}
	for eventType, want := range wantEvents {
		if events[eventType] != want {
			t.Fatalf("post-cutover event %q count = %d, want %d", eventType, events[eventType], want)
		}
	}
	var streamAtLatest bool
	if err := source.Pool.QueryRow(ctx, `SELECT
(SELECT last_event_id FROM teldrive.user_event_stream_state WHERE user_id=101) =
(SELECT max(id) FROM teldrive.user_events WHERE user_id=101)`).Scan(&streamAtLatest); err != nil {
		t.Fatalf("inspect post-cutover event stream state: %v", err)
	}
	if !streamAtLatest {
		t.Fatal("post-cutover event stream state did not advance to the latest event")
	}
	if _, migrated, err := legacymigrate.MigrateIfNeeded(ctx, database.Config{URL: source.URL}, "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", verifier); err != nil || migrated {
		t.Fatalf("second migration = migrated %v, error %v", migrated, err)
	}
}

type verifierFunc func(context.Context, string) (bots.Identity, error)

func (f verifierFunc) Verify(ctx context.Context, token string) (bots.Identity, error) {
	return f(ctx, token)
}
