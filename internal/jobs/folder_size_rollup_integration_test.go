//go:build integration

package jobs_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/riverqueue/river"

	"github.com/tgdrive/teldrive/v2/internal/db/sqlcgen"
	"github.com/tgdrive/teldrive/v2/internal/jobs"
	testpostgres "github.com/tgdrive/teldrive/v2/internal/testutil/postgres"
)

func TestFolderSizeRollupWholeTree(t *testing.T) {
	db := testpostgres.New(t)
	ctx := context.Background()

	folderA := uuid.New()
	folderB := uuid.New()
	folderC := uuid.New()
	if _, err := db.Pool.Exec(ctx, `INSERT INTO users (user_id) VALUES (1001)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Pool.Exec(ctx, `
INSERT INTO files (id, user_id, name, kind, mime_type, size, status, mod_time, updated_at, deleted_at)
VALUES
    ($1, 1001, 'a', 'folder', 'inode/directory', 0, 'active', now(), now(), NULL),
    ($2, 1001, 'b', 'folder', 'inode/directory', 0, 'active', now(), now(), NULL),
    ($3, 1001, 'c', 'folder', 'inode/directory', 0, 'active', now(), now(), NULL)
`, folderA, folderB, folderC); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Pool.Exec(ctx, `UPDATE files SET parent_id = $1 WHERE id = $2`, folderA, folderB); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Pool.Exec(ctx, `
INSERT INTO files (user_id, parent_id, name, kind, mime_type, size, status, mod_time, updated_at, deleted_at)
VALUES
    (1001, $1, 'f1.bin', 'file', 'application/octet-stream', 100, 'active', now(), now(), NULL),
    (1001, $2, 'f2.bin', 'file', 'application/octet-stream', 200, 'active', now(), now(), NULL),
    (1001, $1, 'trashed.bin', 'file', 'application/octet-stream', 999, 'trashed', now(), now(), now())
`, folderA, folderB); err != nil {
		t.Fatal(err)
	}

	worker := jobs.NewFolderSizeRollupWorker(db.Pool)
	if err := worker.Work(ctx, &river.Job[jobs.FolderSizeRollupArgs]{Args: jobs.FolderSizeRollupArgs{}}); err != nil {
		t.Fatalf("Work() error = %v", err)
	}

	sizes := map[uuid.UUID]int64{}
	rows, err := db.Pool.Query(ctx, `SELECT id, size FROM files WHERE kind = 'folder'`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var id uuid.UUID
		var size *int64
		if err := rows.Scan(&id, &size); err != nil {
			t.Fatal(err)
		}
		if size == nil {
			t.Fatalf("folder %s has NULL size after rollup", id)
		}
		sizes[id] = *size
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if sizes[folderA] != 300 {
		t.Fatalf("folder A size = %d, want 300", sizes[folderA])
	}
	if sizes[folderB] != 200 {
		t.Fatalf("folder B size = %d, want 200", sizes[folderB])
	}
	if sizes[folderC] != 0 {
		t.Fatalf("empty folder C size = %d, want 0", sizes[folderC])
	}

	// Second run must be a stable no-op.
	queries := sqlcgen.New(db.Pool)
	affected, err := queries.RollupFolderSizes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if affected != 0 {
		t.Fatalf("second rollup affected %d rows, want 0", affected)
	}
}
