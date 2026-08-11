//go:build integration

package fileops

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/tgdrive/teldrive/v2/internal/catalog"
	"github.com/tgdrive/teldrive/v2/internal/channels"
	"github.com/tgdrive/teldrive/v2/internal/db/sqlcgen"
	testpostgres "github.com/tgdrive/teldrive/v2/internal/testutil/postgres"
)

func TestCopyConflictPolicies(t *testing.T) {
	db := testpostgres.New(t)
	ctx := context.Background()
	if _, err := db.Pool.Exec(ctx, "INSERT INTO users (user_id) VALUES (1001)"); err != nil {
		t.Fatal(err)
	}

	sourceID := insertFolder(t, db, nil, "source")
	failConflictID := insertFolder(t, db, nil, "fail-target")
	renameConflictID := insertFolder(t, db, nil, "rename-target")
	replaceConflictID := insertFolder(t, db, nil, "replace-target")
	replaceChildID := insertFolder(t, db, &replaceConflictID, "replace-child")

	catalogService := catalog.NewService(db.Pool)
	channelService := channels.NewService(db.Pool, nil, channels.Config{PartLimit: 100})
	service, err := NewService(db.Pool, catalogService, channelService, &fileStorage{messages: map[int64][]byte{}})
	if err != nil {
		t.Fatal(err)
	}

	t.Run("fail", func(t *testing.T) {
		name := "fail-target"
		_, err := service.Copy(ctx, CopyInput{
			UserID: 1001, FileID: sourceID, Name: &name,
			ConflictPolicy: sqlcgen.NameConflictPolicyFail,
		})
		if !errors.Is(err, catalog.ErrConflict) {
			t.Fatalf("Copy() error = %v, want catalog.ErrConflict", err)
		}
		assertFileStatus(t, db, failConflictID, sqlcgen.FileStatusActive)
		assertActiveNameCount(t, db, "fail-target", 1)
	})

	t.Run("rename", func(t *testing.T) {
		name := "rename-target"
		copied, err := service.Copy(ctx, CopyInput{
			UserID: 1001, FileID: sourceID, Name: &name,
			ConflictPolicy: sqlcgen.NameConflictPolicyRename,
		})
		if err != nil {
			t.Fatalf("Copy() error = %v", err)
		}
		if copied.Name != "rename-target (1)" || copied.NormalizedName != "rename-target (1)" {
			t.Fatalf("renamed copy = %q / %q", copied.Name, copied.NormalizedName)
		}
		assertFileStatus(t, db, renameConflictID, sqlcgen.FileStatusActive)
		assertActiveNameCount(t, db, "rename-target", 1)
		assertActiveNameCount(t, db, "rename-target (1)", 1)
	})

	t.Run("replace", func(t *testing.T) {
		name := "replace-target"
		copied, err := service.Copy(ctx, CopyInput{
			UserID: 1001, FileID: sourceID, Name: &name,
			ConflictPolicy: sqlcgen.NameConflictPolicyReplace,
		})
		if err != nil {
			t.Fatalf("Copy() error = %v", err)
		}
		if copied.Name != "replace-target" || copied.Status != sqlcgen.FileStatusActive {
			t.Fatalf("replacement copy = %#v", copied)
		}
		assertFileStatus(t, db, replaceConflictID, sqlcgen.FileStatusDeletionPending)
		assertFileStatus(t, db, replaceChildID, sqlcgen.FileStatusDeletionPending)
		assertActiveNameCount(t, db, "replace-target", 1)
	})
}

func insertFolder(t testing.TB, db *testpostgres.Database, parentID *uuid.UUID, name string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	if _, err := db.Pool.Exec(context.Background(), `
INSERT INTO files (id,user_id,parent_id,name,normalized_name,kind,encryption,status,mod_time)
VALUES ($1,1001,$2,$3,lower($3),'folder',false,'active',now())`, id, parentID, name); err != nil {
		t.Fatal(err)
	}
	return id
}

func assertFileStatus(t testing.TB, db *testpostgres.Database, id uuid.UUID, want sqlcgen.FileStatus) {
	t.Helper()
	var status sqlcgen.FileStatus
	if err := db.Pool.QueryRow(context.Background(), "SELECT status FROM files WHERE id=$1", id).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != want {
		t.Fatalf("file %s status = %s, want %s", id, status, want)
	}
}

func assertActiveNameCount(t testing.TB, db *testpostgres.Database, normalizedName string, want int) {
	t.Helper()
	var count int
	if err := db.Pool.QueryRow(context.Background(), `
SELECT count(*) FROM files
WHERE user_id=1001 AND parent_id IS NULL AND normalized_name=$1 AND status='active'`, normalizedName).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != want {
		t.Fatalf("active name %q count = %d, want %d", normalizedName, count, want)
	}
}
