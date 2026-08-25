//go:build integration

package shares

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/tgdrive/teldrive/v2/internal/catalog"
	"github.com/tgdrive/teldrive/v2/internal/db/sqlcgen"
	"github.com/tgdrive/teldrive/v2/internal/dbtypes"
	testpostgres "github.com/tgdrive/teldrive/v2/internal/testutil/postgres"
)

func TestShareLifecycleAndDownloadLimitAgainstRealPostgres(t *testing.T) {
	db := testpostgres.New(t)
	ctx := context.Background()
	fileID := uuid.New()
	if _, err := db.Pool.Exec(ctx, "INSERT INTO users (user_id) VALUES (1001)"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Pool.Exec(ctx, `
INSERT INTO files (id,user_id,name,normalized_name,kind,mime_type,size,encryption,status,mod_time)
VALUES ($1,1001,'shared.bin','shared.bin','file','application/octet-stream',4,false,'active',now())`, fileID); err != nil {
		t.Fatal(err)
	}
	service, err := NewService(db.Pool, catalog.NewService(db.Pool, nil))
	if err != nil {
		t.Fatal(err)
	}
	service.random = bytes.NewReader(bytes.Join([][]byte{bytes.Repeat([]byte{1}, 32), bytes.Repeat([]byte{2}, 32), bytes.Repeat([]byte{3}, 32)}, nil))
	service.now = func() time.Time { return time.Date(2099, 7, 1, 12, 0, 0, 0, time.UTC) }
	password := "correct-password"
	max := int64(1)
	expires := service.now().Add(time.Hour)
	created, err := service.Create(ctx, CreateInput{
		OwnerID: 1001, FileID: fileID, Password: &password, ExpiresAt: &expires, MaxDownloads: &max,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if !strings.HasPrefix(created.Token, "tds_") || !strings.Contains(created.PublicURL.String(), created.Token) {
		t.Fatalf("created share = %#v", created)
	}
	if created.PublicURL.IsAbs() || !strings.HasPrefix(created.PublicURL.Path, "/share/") {
		t.Fatalf("public URL = %q, want relative frontend share URL", created.PublicURL.String())
	}
	if created.Row.PasswordHash.String == password || !created.Row.PasswordHash.Valid {
		t.Fatalf("password hash = %#v", created.Row.PasswordHash)
	}
	if _, err := service.Resolve(ctx, created.Token, "wrong"); !errors.Is(err, ErrInvalidPassword) {
		t.Fatalf("wrong password error = %v", err)
	}
	resolved, err := service.Resolve(ctx, created.Token, password)
	if err != nil || resolved.File.Name != "shared.bin" {
		t.Fatalf("Resolve() = %#v, %v", resolved, err)
	}
	if _, err := service.ReserveDownload(ctx, created.Token, password); err != nil {
		t.Fatalf("ReserveDownload() error = %v", err)
	}
	if _, err := service.ReserveDownload(ctx, created.Token, password); !errors.Is(err, ErrExpired) {
		t.Fatalf("second reservation error = %v", err)
	}
	second, err := service.Create(ctx, CreateInput{OwnerID: 1001, FileID: fileID})
	if err != nil {
		t.Fatalf("Create(second) error = %v", err)
	}
	defaultRows, err := service.List(ctx, ListInput{OwnerID: 1001, FileID: fileID})
	if err != nil || len(defaultRows) != 2 {
		t.Fatalf("List(default) = %#v, %v", defaultRows, err)
	}
	rows, err := service.List(ctx, ListInput{OwnerID: 1001, FileID: fileID, Limit: 10})
	if err != nil || len(rows) != 2 {
		t.Fatalf("List() = %#v, %v", rows, err)
	}
	page, err := service.List(ctx, ListInput{OwnerID: 1001, FileID: fileID, Limit: 1})
	if err != nil || len(page) != 1 {
		t.Fatalf("List(page) = %#v, %v", page, err)
	}
	pageID, _ := dbtypes.GoogleUUID(page[0].ID)
	pageTime := page[0].CreatedAt.Time
	nextPage, err := service.List(ctx, ListInput{OwnerID: 1001, FileID: fileID, AfterID: &pageID, AfterCreatedAt: &pageTime, Limit: 500})
	if err != nil || len(nextPage) != 1 {
		t.Fatalf("List(next) = %#v, %v", nextPage, err)
	}
	shareID, _ := dbtypes.GoogleUUID(created.Row.ID)
	secondID, _ := dbtypes.GoogleUUID(second.Row.ID)
	if err := service.Revoke(ctx, 1001, secondID); err != nil {
		t.Fatalf("Revoke(second) error = %v", err)
	}
	if err := service.Revoke(ctx, 1001, shareID); err != nil {
		t.Fatalf("Revoke() error = %v", err)
	}
	if err := service.Revoke(ctx, 1001, shareID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second Revoke() error = %v", err)
	}
}

func TestShareUpdateAndPublicFolderTraversalAgainstRealPostgres(t *testing.T) {
	db := testpostgres.New(t)
	ctx := context.Background()
	if _, err := db.Pool.Exec(ctx, "INSERT INTO users (user_id) VALUES (1001)"); err != nil {
		t.Fatal(err)
	}
	rootID := uuid.New()
	childFolderID := uuid.New()
	fileID := uuid.New()
	if _, err := db.Pool.Exec(ctx, `
INSERT INTO files (id,user_id,parent_id,name,normalized_name,kind,mime_type,size,encryption,status,mod_time)
VALUES
  ($1,1001,NULL,'Public','Public','folder',NULL,NULL,false,'active',now()),
  ($2,1001,$1,'Docs','Docs','folder',NULL,NULL,false,'active',now()),
  ($3,1001,$2,'Readme.txt','Readme.txt','file','text/plain',6,false,'active',now())`,
		rootID, childFolderID, fileID); err != nil {
		t.Fatal(err)
	}
	service, err := NewService(db.Pool, catalog.NewService(db.Pool, nil))
	if err != nil {
		t.Fatal(err)
	}
	service.random = bytes.NewReader(bytes.Repeat([]byte{7}, 32))
	service.now = func() time.Time { return time.Date(2099, 7, 1, 12, 0, 0, 0, time.UTC) }
	created, err := service.Create(ctx, CreateInput{OwnerID: 1001, FileID: rootID})
	if err != nil {
		t.Fatalf("Create(folder share) error = %v", err)
	}
	shareID, _ := dbtypes.GoogleUUID(created.Row.ID)
	password := "new-password"
	expires := service.now().Add(2 * time.Hour)
	maxDownloads := int64(5)
	updated, err := service.Update(ctx, UpdateInput{
		OwnerID: 1001, ShareID: shareID, Password: &password,
		ExpiresAt: &expires, MaxDownloads: &maxDownloads,
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if !updated.PasswordHash.Valid || !updated.ExpiresAt.Valid || !updated.MaxDownloads.Valid {
		t.Fatalf("updated share = %#v", updated)
	}
	if _, err := service.Resolve(ctx, created.Token, "wrong"); !errors.Is(err, ErrInvalidPassword) {
		t.Fatalf("Resolve(wrong updated password) error = %v", err)
	}
	rootChildren, err := service.ListPublicFiles(ctx, PublicListInput{
		Token: created.Token, Password: password, Limit: 10,
	})
	if err != nil || len(rootChildren) != 1 || rootChildren[0].Name != "Docs" {
		t.Fatalf("ListPublicFiles(root) = %#v, %v", rootChildren, err)
	}
	docsChildren, err := service.ListPublicFiles(ctx, PublicListInput{
		Token: created.Token, Password: password, Path: "Docs", Search: "read", Limit: 10,
	})
	if err != nil || len(docsChildren) != 1 || docsChildren[0].Name != "Readme.txt" {
		t.Fatalf("ListPublicFiles(Docs) = %#v, %v", docsChildren, err)
	}
	if _, err := service.ListPublicFiles(ctx, PublicListInput{
		Token: created.Token, Password: password, Path: "../Private", Limit: 10,
	}); !errors.Is(err, catalog.ErrInvalidParent) {
		t.Fatalf("path escape error = %v", err)
	}
	cleared, err := service.Update(ctx, UpdateInput{
		OwnerID: 1001, ShareID: shareID, ClearPassword: true,
		ClearExpiresAt: true, ClearMaxDownloads: true,
	})
	if err != nil {
		t.Fatalf("Update(clear) error = %v", err)
	}
	if cleared.PasswordHash.Valid || cleared.ExpiresAt.Valid || cleared.MaxDownloads.Valid {
		t.Fatalf("cleared share = %#v", cleared)
	}
	if _, err := service.Resolve(ctx, created.Token, ""); err != nil {
		t.Fatalf("Resolve(after clear) error = %v", err)
	}
}

func TestInternalGrantAccessLifecycleAgainstRealPostgres(t *testing.T) {
	db := testpostgres.New(t)
	ctx := context.Background()
	if _, err := db.Pool.Exec(ctx, "INSERT INTO users (user_id) VALUES (1001), (2002), (3003)"); err != nil {
		t.Fatal(err)
	}
	rootID, childID := uuid.New(), uuid.New()
	if _, err := db.Pool.Exec(ctx, `
INSERT INTO files (id,user_id,parent_id,name,normalized_name,kind,mime_type,size,encryption,status,mod_time)
VALUES
  ($1,1001,NULL,'Team','Team','folder',NULL,NULL,false,'active',now()),
  ($2,1001,$1,'notes.txt','notes.txt','file','text/plain',5,false,'active',now())`, rootID, childID); err != nil {
		t.Fatal(err)
	}
	service, err := NewService(db.Pool, catalog.NewService(db.Pool, nil))
	if err != nil {
		t.Fatal(err)
	}
	service.now = func() time.Time { return time.Date(2099, 7, 1, 12, 0, 0, 0, time.UTC) }

	grant, err := service.CreateGrant(ctx, GrantCreateInput{
		OwnerID: 1001, FileID: rootID, GranteeID: 2002, Permission: sqlcgen.SharePermissionRead,
	})
	if err != nil {
		t.Fatalf("CreateGrant() error = %v", err)
	}
	access, err := service.ResolveAccess(ctx, 2002, childID, false)
	if err != nil || access.OwnerID != 1001 || access.RootFileID != rootID || access.Permission != sqlcgen.SharePermissionRead {
		t.Fatalf("ResolveAccess(read) = %#v, %v", access, err)
	}
	if _, err := service.ResolveAccess(ctx, 2002, childID, true); !errors.Is(err, ErrForbidden) {
		t.Fatalf("ResolveAccess(edit with read grant) error = %v", err)
	}
	shared, err := service.ListSharedWithMe(ctx, 2002)
	if err != nil || len(shared) != 1 || shared[0].OwnerID != 1001 {
		t.Fatalf("ListSharedWithMe() = %#v, %v", shared, err)
	}

	grantID, _ := dbtypes.GoogleUUID(grant.ID)
	editPermission := sqlcgen.SharePermissionEdit
	if _, err := service.UpdateGrant(ctx, GrantUpdateInput{OwnerID: 1001, GrantID: grantID, Permission: &editPermission}); err != nil {
		t.Fatalf("UpdateGrant(edit) error = %v", err)
	}
	if access, err := service.ResolveAccess(ctx, 2002, childID, true); err != nil || access.Permission != sqlcgen.SharePermissionEdit {
		t.Fatalf("ResolveAccess(edit) = %#v, %v", access, err)
	}
	if err := service.RevokeGrant(ctx, 1001, grantID); err != nil {
		t.Fatalf("RevokeGrant() error = %v", err)
	}
	if _, err := service.ResolveAccess(ctx, 2002, childID, false); !errors.Is(err, ErrForbidden) {
		t.Fatalf("ResolveAccess(after revoke) error = %v", err)
	}

	if _, err := db.Pool.Exec(ctx, "UPDATE users SET disabled_at = now() WHERE user_id = 3003"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateGrant(ctx, GrantCreateInput{OwnerID: 1001, FileID: rootID, GranteeID: 3003}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("CreateGrant(disabled recipient) error = %v", err)
	}
}

func TestPublicShareEditPermissionAgainstRealPostgres(t *testing.T) {
	db := testpostgres.New(t)
	ctx := context.Background()
	if _, err := db.Pool.Exec(ctx, "INSERT INTO users (user_id) VALUES (1001)"); err != nil {
		t.Fatal(err)
	}
	rootID, childID := uuid.New(), uuid.New()
	if _, err := db.Pool.Exec(ctx, `
INSERT INTO files (id,user_id,parent_id,name,normalized_name,kind,mime_type,size,encryption,status,mod_time)
VALUES
  ($1,1001,NULL,'Public','Public','folder',NULL,NULL,false,'active',now()),
  ($2,1001,$1,'child.txt','child.txt','file','text/plain',5,false,'active',now())`, rootID, childID); err != nil {
		t.Fatal(err)
	}
	service, err := NewService(db.Pool, catalog.NewService(db.Pool, nil))
	if err != nil {
		t.Fatal(err)
	}
	service.random = bytes.NewReader(bytes.Join([][]byte{bytes.Repeat([]byte{9}, 32), bytes.Repeat([]byte{10}, 32)}, nil))
	service.now = func() time.Time { return time.Date(2099, 7, 1, 12, 0, 0, 0, time.UTC) }

	readShare, err := service.Create(ctx, CreateInput{OwnerID: 1001, FileID: rootID, Permission: sqlcgen.SharePermissionRead})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ResolvePublicEditableFile(ctx, readShare.Token, "", childID); !errors.Is(err, ErrForbidden) {
		t.Fatalf("read share edit error = %v", err)
	}

	editShare, err := service.Create(ctx, CreateInput{OwnerID: 1001, FileID: rootID, Permission: sqlcgen.SharePermissionEdit})
	if err != nil {
		t.Fatal(err)
	}
	if resolved, err := service.ResolvePublicEditableFile(ctx, editShare.Token, "", childID); err != nil || resolved.Share.Permission != sqlcgen.SharePermissionEdit {
		t.Fatalf("edit share resolve = %#v, %v", resolved, err)
	}
	if _, parentID, err := service.ResolvePublicEditableParent(ctx, editShare.Token, "", nil); err != nil || parentID != rootID {
		t.Fatalf("edit share parent = %v, %v", parentID, err)
	}
}
