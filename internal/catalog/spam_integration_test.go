//go:build integration

package catalog_test

import (
	"context"
	"github.com/google/uuid"
	"github.com/tgdrive/teldrive/v2/internal/catalog"
	"github.com/tgdrive/teldrive/v2/internal/db/sqlcgen"
	"github.com/tgdrive/teldrive/v2/internal/dbtypes"
	testpostgres "github.com/tgdrive/teldrive/v2/internal/testutil/postgres"
	"testing"
	"time"
)

func TestSpamSubtreeIsolationRestorationAndExpiry(t *testing.T) {
	db := testpostgres.New(t)
	ctx := context.Background()
	seedUser(t, db.Pool, 1001)
	seedUser(t, db.Pool, 2002)
	svc := catalog.NewService(db.Pool, nil)
	folder, err := svc.CreateFolder(ctx, catalog.CreateFolderInput{UserID: 1001, Name: "Suspicious"})
	if err != nil {
		t.Fatal(err)
	}
	root := mustUUID(t, folder.ID)
	child := seedFile(t, db.Pool, 1001, &root, "audio.wav", "audio/wav", 10, time.Now())
	foreign := seedFile(t, db.Pool, 2002, nil, "foreign.txt", "text/plain", 1, time.Now())
	if _, err := svc.BulkSpam(ctx, 1001, []uuid.UUID{root, foreign}); err == nil {
		t.Fatal("cross-owner spam succeeded")
	}
	active, err := svc.Get(ctx, 1001, root)
	if err != nil || active.Status != sqlcgen.FileStatusActive {
		t.Fatal("failed bulk request partially quarantined root")
	}
	if _, err := db.Pool.Exec(ctx, "INSERT INTO file_access_grants(file_id,owner_id,grantee_id,permission) VALUES($1,1001,2002,'read')", root); err != nil {
		t.Fatal(err)
	}
	items, err := svc.BulkSpam(ctx, 1001, []uuid.UUID{root})
	if err != nil || len(items) != 2 {
		t.Fatalf("quarantine: %d %v", len(items), err)
	}
	for _, id := range []uuid.UUID{root, child} {
		file, err := svc.Get(ctx, 1001, id)
		if err != nil || file.Status != sqlcgen.FileStatusSpam || !file.DeletedAt.Valid {
			t.Fatalf("spam status: %v %v", file, err)
		}
	}
	listed, err := svc.List(ctx, catalog.ListInput{UserID: 1001, Status: sqlcgen.FileStatusSpam, Sort: "updatedAt", Order: "desc"})
	if err != nil || len(listed) != 1 {
		t.Fatalf("root listing: %d %v", len(listed), err)
	}
	var revoked bool
	if err := db.Pool.QueryRow(ctx, "SELECT revoked_at IS NOT NULL FROM file_access_grants WHERE file_id=$1", root).Scan(&revoked); err != nil || !revoked {
		t.Fatal("spam did not revoke incoming access")
	}
	if _, err := svc.Restore(ctx, 1001, root); err != nil {
		t.Fatal(err)
	}
	restored, err := svc.Get(ctx, 1001, child)
	if err != nil || restored.Status != sqlcgen.FileStatusActive || restored.DeletedAt.Valid {
		t.Fatal("child not restored")
	}
	if _, err := svc.BulkSpam(ctx, 1001, []uuid.UUID{root}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Pool.Exec(ctx, "UPDATE files SET deleted_at=now()-interval '31 days' WHERE id=ANY($1::uuid[])", []uuid.UUID{root, child}); err != nil {
		t.Fatal(err)
	}
	expired, err := sqlcgen.New(db.Pool).ListTrashedRootsBefore(ctx, dbtypes.Time(time.Now().Add(-30*24*time.Hour)))
	if err != nil || len(expired) != 1 {
		t.Fatalf("spam expiry: %d %v", len(expired), err)
	}
}
