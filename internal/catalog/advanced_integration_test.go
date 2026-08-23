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
INSERT INTO files (id, user_id, parent_id, name, normalized_name, kind, mime_type, size, status, mod_time, updated_at, deleted_at)
VALUES
    ($1, 1001, NULL, 'Photos', 'Photos', 'folder', NULL, NULL, 'active', now(), now() - interval '4 minutes', NULL),
    ($2, 1001, $1, 'Nested', 'Nested', 'folder', NULL, NULL, 'active', now(), now() - interval '3 minutes', NULL),
    ($3, 1001, $2, 'sunset.JPG', 'sunset.JPG', 'file', 'image/jpeg', 20, 'active', now(), now() - interval '2 minutes', NULL),
    ($4, 1001, $2, 'notes.txt', 'notes.txt', 'file', 'text/plain', 10, 'active', now(), now() - interval '1 minute', NULL),
    ($5, 1001, NULL, 'old.zip', 'old.zip', 'file', 'application/zip', 30, 'trashed', now(), now(), now())
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
