//go:build integration

package shares_test

import (
	"context"
	"github.com/google/uuid"
	"github.com/tgdrive/teldrive/v2/internal/catalog"
	"github.com/tgdrive/teldrive/v2/internal/db/sqlcgen"
	"github.com/tgdrive/teldrive/v2/internal/dbtypes"
	"github.com/tgdrive/teldrive/v2/internal/shares"
	testpostgres "github.com/tgdrive/teldrive/v2/internal/testutil/postgres"
	"testing"
	"time"
)

func TestIncomingSpamIsPersonalAndExpiryPreservesOwnerFiles(t *testing.T) {
	db := testpostgres.New(t)
	ctx := context.Background()
	root, child := uuid.New(), uuid.New()
	if _, err := db.Pool.Exec(ctx, `INSERT INTO users(user_id) VALUES(1001),(1002),(1003)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Pool.Exec(ctx, `INSERT INTO files(id,user_id,parent_id,name,kind,size,status,mod_time) VALUES($1,1001,NULL,'shared','folder',NULL,'active',now()),($2,1001,$1,'child','file',0,'active',now());`, root, child); err != nil {
		t.Fatal(err)
	}
	cat := catalog.NewService(db.Pool, nil)
	svc, err := shares.NewService(db.Pool, cat)
	if err != nil {
		t.Fatal(err)
	}
	for _, actor := range []int64{1002, 1003} {
		if _, err := svc.CreateGrant(ctx, shares.GrantCreateInput{OwnerID: 1001, GranteeID: actor, FileID: root}); err != nil {
			t.Fatal(err)
		}
	}
	if err := svc.MarkIncomingSpam(ctx, 1002, root); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ResolveAccess(ctx, 1002, child, false); err == nil {
		t.Fatal("child remains accessible to reporting recipient")
	}
	if _, err := svc.ResolveAccess(ctx, 1003, child, false); err != nil {
		t.Fatalf("other recipient affected: %v", err)
	}
	if _, err := svc.ResolveAccess(ctx, 1001, child, true); err != nil {
		t.Fatalf("owner affected: %v", err)
	}
	visible, err := svc.ListSharedWithMe(ctx, 1002)
	if err != nil || len(visible) != 0 {
		t.Fatalf("incoming list: %d %v", len(visible), err)
	}
	if err := svc.RestoreIncomingSpam(ctx, 1002, root, false); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ResolveAccess(ctx, 1002, child, false); err != nil {
		t.Fatalf("restore did not restore access: %v", err)
	}
	if err := svc.MarkIncomingSpam(ctx, 1002, root); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Pool.Exec(ctx, "UPDATE incoming_spam SET reported_at=now()-interval '31 days' WHERE grantee_id=1002"); err != nil {
		t.Fatal(err)
	}
	if err := sqlcgen.New(db.Pool).ExpireIncomingSpam(ctx, dbtypes.Time(time.Now().Add(-30*24*time.Hour))); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ResolveAccess(ctx, 1002, child, false); err == nil {
		t.Fatal("expired spam reopened access")
	}
	original, err := cat.Get(ctx, 1001, child)
	if err != nil || original.Status != sqlcgen.FileStatusActive {
		t.Fatal("expiry deleted or trashed owner's original")
	}
}
