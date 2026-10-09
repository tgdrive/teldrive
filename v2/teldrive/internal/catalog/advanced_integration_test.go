//go:build integration

package catalog_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/tgdrive/teldrive/v2/internal/catalog"
	"github.com/tgdrive/teldrive/v2/internal/db/sqlcgen"
	"github.com/tgdrive/teldrive/v2/internal/dbtypes"
	testpostgres "github.com/tgdrive/teldrive/v2/internal/testutil/postgres"
)

func TestAdvancedListingPathAndStatistics(t *testing.T) {
	db := testpostgres.New(t)
	ctx := context.Background()
	if _, err := db.Pool.Exec(ctx, "INSERT INTO users (user_id) VALUES (1001)"); err != nil {
		t.Fatal(err)
	}
	folderID := uuid.New()
	nestedID := uuid.New()
	imageID := uuid.New()
	docID := uuid.New()
	trashedID := uuid.New()
	if _, err := db.Pool.Exec(ctx, `
INSERT INTO files (id, user_id, parent_id, name, kind, mime_type, size, status, mod_time, updated_at, deleted_at)
VALUES
    ($1, 1001, NULL, 'Photos', 'folder', NULL, NULL, 'active', now(), now() - interval '4 minutes', NULL),
    ($2, 1001, $1, 'Nested', 'folder', NULL, NULL, 'active', now(), now() - interval '3 minutes', NULL),
    ($3, 1001, $2, 'sunset.JPG', 'file', 'image/jpeg', 20, 'active', now(), now() - interval '2 minutes', NULL),
    ($4, 1001, $2, 'notes.txt', 'file', 'text/plain', 10, 'active', now(), now() - interval '1 minute', NULL),
    ($5, 1001, NULL, 'old.zip', 'file', 'application/zip', 30, 'trashed', now(), now(), now())
`, folderID, nestedID, imageID, docID, trashedID); err != nil {
		t.Fatal(err)
	}

	svc := catalog.NewService(db.Pool, nil)
	resolved, err := svc.ResolveFolderPath(ctx, 1001, nil, " /Photos/Nested/ ")
	if err != nil || resolved == nil || *resolved != nestedID {
		t.Fatalf("ResolveFolderPath() = %v, %v", resolved, err)
	}
	if empty, err := svc.ResolveFolderPath(ctx, 1001, nil, ""); err != nil || empty != nil {
		t.Fatalf("empty ResolveFolderPath() = %v, %v", empty, err)
	}
	if _, err := svc.ResolveFolderPath(ctx, 1001, nil, "Photos/../Nested"); !errors.Is(err, catalog.ErrInvalidParent) {
		t.Fatalf("invalid path error = %v", err)
	}
	kind := sqlcgen.FileKindFile
	items, err := svc.List(ctx, catalog.ListInput{
		UserID: 1001, Path: "Photos/Nested", Kind: &kind,
		Search: "sun", SearchType: "regex", Categories: []string{"image"},
		Sort: "size", Order: "desc", Limit: 10,
	})
	if err != nil {
		t.Fatalf("advanced List() error = %v", err)
	}
	if len(items) != 1 || items[0].Name != "sunset.JPG" {
		t.Fatalf("advanced List() = %#v", items)
	}
	trashed, err := svc.List(ctx, catalog.ListInput{
		UserID: 1001, Status: sqlcgen.FileStatusTrashed,
		SearchType: "text", Sort: "updatedAt", Order: "desc", Limit: 10,
	})
	if err != nil || len(trashed) != 1 || trashed[0].Name != "old.zip" {
		t.Fatalf("advanced trashed List() = %#v, %v", trashed, err)
	}

	if got := catalog.FileCursorValue(items[0], "size"); got != "20" {
		t.Fatalf("size cursor = %q", got)
	}
	if got := catalog.FileCursorValue(items[0], "updatedAt"); got == "" {
		t.Fatal("updatedAt cursor is empty")
	}
	if got := catalog.FileCursorValue(items[0], "name"); got != "sunset.JPG" {
		t.Fatalf("name cursor = %q", got)
	}
	if got := catalog.FileCursorValue(nil, "name"); got != "" {
		t.Fatalf("nil cursor = %q", got)
	}

	invalidInputs := []catalog.ListInput{
		{UserID: 1001, SearchType: "bad", Sort: "name", Order: "asc"},
		{UserID: 1001, SearchType: "regex", Search: "[", Sort: "name", Order: "asc"},
		{UserID: 1001, SearchType: "text", Sort: "bad", Order: "asc", Categories: []string{"image"}},
		{UserID: 1001, SearchType: "text", Sort: "name", Order: "bad", Categories: []string{"image"}},
		{UserID: 1001, SearchType: "text", Sort: "name", Order: "asc", Categories: []string{"invalid"}},
	}
	for _, input := range invalidInputs {
		if _, err := svc.List(ctx, input); !errors.Is(err, catalog.ErrInvalidParent) {
			t.Fatalf("invalid advanced List(%#v) error = %v", input, err)
		}
	}
	after := time.Now()
	before := after.Add(-time.Hour)
	if _, err := svc.List(ctx, catalog.ListInput{
		UserID: 1001, SearchType: "text", Sort: "name", Order: "asc",
		Categories: []string{"image"}, UpdatedAfter: &after, UpdatedBefore: &before,
	}); !errors.Is(err, catalog.ErrInvalidParent) {
		t.Fatalf("inverted time range error = %v", err)
	}

	categories, err := svc.CategoryStatistics(ctx, 1001)
	if err != nil {
		t.Fatalf("CategoryStatistics() error = %v", err)
	}
	if len(categories) < 2 {
		t.Fatalf("CategoryStatistics() = %#v", categories)
	}
	drive, err := svc.DriveStatistics(ctx, 1001)
	if err != nil {
		t.Fatalf("DriveStatistics() error = %v", err)
	}
	if drive.TotalFiles != 2 || drive.TotalFolders != 2 || drive.TotalBytes != 30 || drive.TrashedFiles != 1 {
		t.Fatalf("DriveStatistics() = %#v", drive)
	}
	dashboard, err := svc.StorageDashboard(ctx, 1001)
	if err != nil {
		t.Fatalf("StorageDashboard() error = %v", err)
	}
	if dashboard.Summary.LogicalBytes != 30 || dashboard.Summary.TrashBytes != 30 || len(dashboard.Growth) != 30 {
		t.Fatalf("StorageDashboard() = %#v", dashboard)
	}
	if _, err := svc.StorageDashboard(ctx, 0); !errors.Is(err, catalog.ErrInvalidOwner) {
		t.Fatalf("invalid storage dashboard owner error = %v", err)
	}
	if _, err := svc.CategoryStatistics(ctx, 0); !errors.Is(err, catalog.ErrInvalidOwner) {
		t.Fatalf("invalid category owner error = %v", err)
	}
	if _, err := svc.DriveStatistics(ctx, 0); !errors.Is(err, catalog.ErrInvalidOwner) {
		t.Fatalf("invalid drive owner error = %v", err)
	}
}

func TestEnsureFolderPathCreatesMissingFolders(t *testing.T) {
	db := testpostgres.New(t)
	ctx := context.Background()
	if _, err := db.Pool.Exec(ctx, "INSERT INTO users (user_id) VALUES (1001)"); err != nil {
		t.Fatal(err)
	}
	svc := catalog.NewService(db.Pool, nil)
	created, err := svc.EnsureFolderPath(ctx, 1001, nil, "/Photos/Uploads/Incoming")
	if err != nil || created == nil {
		t.Fatalf("EnsureFolderPath() = %v, %v", created, err)
	}
	resolved, err := svc.ResolveFolderPath(ctx, 1001, nil, "/Photos/Uploads/Incoming")
	if err != nil || resolved == nil || *resolved != *created {
		t.Fatalf("ResolveFolderPath(created) = %v, %v", resolved, err)
	}
	if _, err := svc.EnsureFolderPath(ctx, 1001, nil, "/Would/Create/../Invalid"); !errors.Is(err, catalog.ErrInvalidParent) {
		t.Fatalf("invalid ensure path error = %v", err)
	}
	if _, err := svc.ResolveFolderPath(ctx, 1001, nil, "/Would"); !errors.Is(err, catalog.ErrInvalidParent) {
		t.Fatalf("invalid ensure path created partial folders: %v", err)
	}
}

func TestScopedListingsIsolationPathsAndValidation(t *testing.T) {
	db := testpostgres.New(t)
	ctx := context.Background()
	if _, err := db.Pool.Exec(ctx, "INSERT INTO users (user_id) VALUES (1001), (1002)"); err != nil {
		t.Fatal(err)
	}
	root, nested, leaf, sibling, inactive, other, secondPDF, inactiveFolder, recovered := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	if _, err := db.Pool.Exec(ctx, `
INSERT INTO files (id,user_id,parent_id,name,kind,mime_type,size,status,mod_time,updated_at,deleted_at) VALUES
($1,1001,NULL,'Docs','folder',NULL,NULL,'active',now(),now(),NULL),
($2,1001,$1,'Reports','folder',NULL,NULL,'active',now(),now(),NULL),
($3,1001,$2,'annual.pdf','file','application/pdf',10,'active',now(),now(),NULL),
($4,1001,$1,'Sibling.txt','file','text/plain',20,'active',now(),now(),NULL),
($5,1001,$2,'gone.pdf','file','application/pdf',30,'trashed',now(),now(),now()),
($6,1002,NULL,'foreign.pdf','file','application/pdf',40,'active',now(),now(),NULL),
($7,1001,$1,'budget.pdf','file','application/pdf',15,'active',now(),now(),NULL),
($8,1001,$1,'Archive','folder',NULL,NULL,'trashed',now(),now(),now()),
($9,1001,$8,'recovered.txt','file','text/plain',12,'active',now(),now(),NULL)
`, root, nested, leaf, sibling, inactive, other, secondPDF, inactiveFolder, recovered); err != nil {
		t.Fatal(err)
	}
	svc := catalog.NewService(db.Pool, nil)
	drive, err := svc.List(ctx, catalog.ListInput{UserID: 1001, Scope: "drive", Status: sqlcgen.FileStatusActive, Sort: "name", Order: "asc", Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	if len(drive) != 6 {
		t.Fatalf("drive returned %d active own rows, want 6", len(drive))
	}
	for _, f := range drive {
		if f.UserID != 1001 || f.Status != sqlcgen.FileStatusActive {
			t.Fatalf("drive leaked/inactive file: %#v", f)
		}
	}
	paths, err := svc.ParentPaths(ctx, 1001, []uuid.UUID{root, nested, leaf, sibling, recovered})
	if err != nil {
		t.Fatal(err)
	}
	if paths[root] != "/" || paths[nested] != "/Docs" || paths[leaf] != "/Docs/Reports" || paths[sibling] != "/Docs" || paths[recovered] != "/Docs/Archive" {
		t.Fatalf("parent paths = %#v", paths)
	}
	recursive, err := svc.List(ctx, catalog.ListInput{UserID: 1001, Scope: "recursive", ScopeFolderID: &root, Status: sqlcgen.FileStatusActive, Sort: "name", Order: "asc", Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	if len(recursive) != 5 {
		t.Fatalf("recursive returned %d rows, want active descendants across inactive parents, excluding root/inactive", len(recursive))
	}
	for _, f := range recursive {
		if f.ID == dbtypes.UUID(root) || f.Status != sqlcgen.FileStatusActive {
			t.Fatalf("recursive scope included selected folder or inactive file: %#v", f)
		}
	}
	nestedRecursive, err := svc.List(ctx, catalog.ListInput{UserID: 1001, Scope: "recursive", ScopeFolderID: &nested, Status: sqlcgen.FileStatusActive, Sort: "name", Order: "asc", Limit: 100})
	if err != nil || len(nestedRecursive) != 1 || nestedRecursive[0].Name != "annual.pdf" {
		t.Fatalf("nested recursive scope = %#v, %v; want only descendant, not selected folder or sibling", nestedRecursive, err)
	}
	page, err := svc.List(ctx, catalog.ListInput{UserID: 1001, Scope: "drive", Status: sqlcgen.FileStatusActive, Kind: func() *sqlcgen.FileKind { k := sqlcgen.FileKindFile; return &k }(), Search: "pdf", Categories: []string{"document"}, Sort: "name", Order: "asc", Limit: 1})
	if err != nil || len(page) != 1 || page[0].Name != "annual.pdf" {
		t.Fatalf("filtered drive page = %#v, %v", page, err)
	}
	pageID, ok := dbtypes.GoogleUUID(page[0].ID)
	if !ok {
		t.Fatal("first page file has invalid ID")
	}
	page2, err := svc.List(ctx, catalog.ListInput{UserID: 1001, Scope: "drive", Status: sqlcgen.FileStatusActive, Kind: func() *sqlcgen.FileKind { k := sqlcgen.FileKindFile; return &k }(), Search: "pdf", Categories: []string{"document"}, Sort: "name", Order: "asc", Limit: 1, AfterID: &pageID, AfterName: page[0].Name, AfterValue: page[0].Name})
	if err != nil || len(page2) != 1 || page2[0].Name != "budget.pdf" {
		t.Fatalf("filtered drive second page = %#v, %v", page2, err)
	}
	if _, err := svc.List(ctx, catalog.ListInput{UserID: 1001, Scope: "recursive", ScopeFolderID: &other, Status: sqlcgen.FileStatusActive}); !errors.Is(err, catalog.ErrInvalidParent) {
		t.Fatalf("foreign recursive folder error = %v", err)
	}
	if _, err := svc.List(ctx, catalog.ListInput{UserID: 1001, Scope: "drive", Status: sqlcgen.FileStatusTrashed}); !errors.Is(err, catalog.ErrInvalidParent) {
		t.Fatalf("drive trashed status error = %v", err)
	}
}
