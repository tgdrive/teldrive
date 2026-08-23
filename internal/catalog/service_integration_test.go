//go:build integration

package catalog_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tgdrive/teldrive/v2/internal/catalog"
	"github.com/tgdrive/teldrive/v2/internal/db/sqlcgen"
	"github.com/tgdrive/teldrive/v2/internal/dbtypes"
	testpostgres "github.com/tgdrive/teldrive/v2/internal/testutil/postgres"
)

func TestCatalogLifecycleAgainstRealPostgres(t *testing.T) {
	db := testpostgres.New(t)
	ctx := context.Background()
	seedUser(t, db.Pool, 1001)
	seedUser(t, db.Pool, 2002)

	svc := catalog.NewService(db.Pool, nil)
	docs, err := svc.CreateFolder(ctx, catalog.CreateFolderInput{UserID: 1001, Name: "Docs"})
	if err != nil {
		t.Fatalf("create root folder: %v", err)
	}
	docsID := mustUUID(t, docs.ID)

	lowerDocs, err := svc.CreateFolder(ctx, catalog.CreateFolderInput{UserID: 1001, Name: "docs"})
	if err != nil {
		t.Fatalf("create case-distinct root folder: %v", err)
	}
	if lowerDocs.NormalizedName != "docs" || docs.NormalizedName != "Docs" {
		t.Fatalf("case-sensitive normalized names = %q, %q", docs.NormalizedName, lowerDocs.NormalizedName)
	}
	if _, err := svc.CreateFolder(ctx, catalog.CreateFolderInput{UserID: 1001, Name: "Docs"}); !errors.Is(err, catalog.ErrConflict) {
		t.Fatalf("exact duplicate error = %v, want ErrConflict", err)
	}
	if _, err := svc.Get(ctx, 2002, docsID); !errors.Is(err, catalog.ErrNotFound) {
		t.Fatalf("cross-owner read error = %v, want ErrNotFound", err)
	}

	reports, err := svc.CreateFolder(ctx, catalog.CreateFolderInput{UserID: 1001, ParentID: &docsID, Name: "Reports"})
	if err != nil {
		t.Fatalf("create reports folder: %v", err)
	}
	reportsID := mustUUID(t, reports.ID)
	archive, err := svc.CreateFolder(ctx, catalog.CreateFolderInput{UserID: 1001, ParentID: &reportsID, Name: "Archive"})
	if err != nil {
		t.Fatalf("create archive folder: %v", err)
	}
	archiveID := mustUUID(t, archive.ID)

	if _, err := svc.Move(ctx, 1001, docsID, &archiveID, nil); !errors.Is(err, catalog.ErrCycle) {
		t.Fatalf("cycle move error = %v, want ErrCycle", err)
	}

	moved, err := svc.Move(ctx, 1001, reportsID, nil, &reports.Generation)
	if err != nil {
		t.Fatalf("move reports to root: %v", err)
	}
	wrongGeneration := moved.Generation - 1
	if _, err := svc.Rename(ctx, 1001, reportsID, &wrongGeneration, "Quarterly"); !errors.Is(err, catalog.ErrPrecondition) {
		t.Fatalf("stale rename error = %v, want ErrPrecondition", err)
	}
	renamed, err := svc.Rename(ctx, 1001, reportsID, &moved.Generation, "Quarterly")
	if err != nil {
		t.Fatalf("rename reports: %v", err)
	}
	if renamed.Name != "Quarterly" || renamed.Generation != moved.Generation+1 {
		t.Fatalf("renamed file = %#v", renamed)
	}

	items, err := svc.List(ctx, catalog.ListInput{UserID: 1001, Limit: 200})
	if err != nil {
		t.Fatalf("list root: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("root listing = %#v", items)
	}
	listedNames := map[string]bool{}
	for _, item := range items {
		listedNames[item.NormalizedName] = true
	}
	for _, name := range []string{"Docs", "docs", "Quarterly"} {
		if !listedNames[name] {
			t.Fatalf("root listing missing %q: %#v", name, items)
		}
	}

	trashed, err := svc.Trash(ctx, 1001, docsID)
	if err != nil {
		t.Fatalf("trash docs: %v", err)
	}
	if trashed.Status != sqlcgen.FileStatusTrashed || !trashed.DeletedAt.Valid {
		t.Fatalf("trashed file = %#v", trashed)
	}
	if _, err := svc.CreateFolder(ctx, catalog.CreateFolderInput{UserID: 1001, Name: "DOCS"}); err != nil {
		t.Fatalf("create case-distinct name after trash: %v", err)
	}
	if restored, err := svc.Restore(ctx, 1001, docsID); err != nil || restored.Name != "Docs" {
		t.Fatalf("restore case-distinct folder = %#v, %v", restored, err)
	}
}

func TestTrashRootListingIncludesNestedDeletedItems(t *testing.T) {
	db := testpostgres.New(t)
	ctx := context.Background()
	seedUser(t, db.Pool, 1001)
	svc := catalog.NewService(db.Pool, nil)

	activeFolder, err := svc.CreateFolder(ctx, catalog.CreateFolderInput{UserID: 1001, Name: "Active"})
	if err != nil {
		t.Fatal(err)
	}
	activeFolderID := mustUUID(t, activeFolder.ID)
	deletedFileID := seedFile(t, db.Pool, 1001, &activeFolderID, "deleted-by-rclone.txt", "text/plain", 10, time.Now())
	if _, err := svc.Trash(ctx, 1001, deletedFileID); err != nil {
		t.Fatalf("trash nested file: %v", err)
	}

	trashedFolder, err := svc.CreateFolder(ctx, catalog.CreateFolderInput{UserID: 1001, Name: "DeletedFolder"})
	if err != nil {
		t.Fatal(err)
	}
	trashedFolderID := mustUUID(t, trashedFolder.ID)
	trashedChildID := seedFile(t, db.Pool, 1001, &trashedFolderID, "child.txt", "text/plain", 20, time.Now())
	if _, err := svc.BulkTrash(ctx, 1001, []uuid.UUID{trashedFolderID}); err != nil {
		t.Fatalf("trash folder subtree: %v", err)
	}

	items, err := svc.List(ctx, catalog.ListInput{UserID: 1001, Status: sqlcgen.FileStatusTrashed, Limit: 100})
	if err != nil {
		t.Fatalf("list trash root: %v", err)
	}
	got := map[uuid.UUID]bool{}
	for _, item := range items {
		got[mustUUID(t, item.ID)] = true
	}
	if !got[deletedFileID] {
		t.Fatalf("trash root missing nested deleted file %s: %#v", deletedFileID, items)
	}
	if !got[trashedFolderID] {
		t.Fatalf("trash root missing trashed folder %s: %#v", trashedFolderID, items)
	}
	if got[trashedChildID] {
		t.Fatalf("trash root unexpectedly includes child of trashed folder %s: %#v", trashedChildID, items)
	}
}

func TestAdvancedListingBulkOperationsAndStatistics(t *testing.T) {
	db := testpostgres.New(t)
	ctx := context.Background()
	seedUser(t, db.Pool, 1001)
	svc := catalog.NewService(db.Pool, nil)

	docs, err := svc.CreateFolder(ctx, catalog.CreateFolderInput{UserID: 1001, Name: "Docs"})
	if err != nil {
		t.Fatal(err)
	}
	docsID := mustUUID(t, docs.ID)
	reports, err := svc.CreateFolder(ctx, catalog.CreateFolderInput{UserID: 1001, ParentID: &docsID, Name: "Reports"})
	if err != nil {
		t.Fatal(err)
	}
	reportsID := mustUUID(t, reports.ID)
	archive, err := svc.CreateFolder(ctx, catalog.CreateFolderInput{UserID: 1001, Name: "Archive"})
	if err != nil {
		t.Fatal(err)
	}
	archiveID := mustUUID(t, archive.ID)

	pdfID := seedFile(t, db.Pool, 1001, &reportsID, "Annual Report.pdf", "application/pdf", 1200, time.Now().Add(-2*time.Hour))
	imageID := seedFile(t, db.Pool, 1001, &reportsID, "Cover.JPG", "image/jpeg", 300, time.Now().Add(-time.Hour))
	_ = imageID
	seedFile(t, db.Pool, 1001, &archiveID, "Annual Report.pdf", "application/pdf", 800, time.Now())

	resolved, err := svc.ResolveFolderPath(ctx, 1001, nil, "/Docs/Reports/")
	if err != nil || resolved == nil || *resolved != reportsID {
		t.Fatalf("ResolveFolderPath() = %v, %v", resolved, err)
	}
	listed, err := svc.List(ctx, catalog.ListInput{
		UserID: 1001, Path: "Docs/Reports", Search: `(?i)report\.pdf$`, SearchType: "regex",
		Categories: []string{"document"}, Sort: "size", Order: "desc", Limit: 10,
	})
	if err != nil || len(listed) != 1 || mustUUID(t, listed[0].ID) != pdfID {
		t.Fatalf("advanced List() = %#v, %v", listed, err)
	}
	categoryStats, err := svc.CategoryStatistics(ctx, 1001)
	if err != nil {
		t.Fatalf("CategoryStatistics() error = %v", err)
	}
	var documentFiles, imageFiles int64
	for _, item := range categoryStats {
		switch item.Category {
		case "document":
			documentFiles = item.TotalFiles
		case "image":
			imageFiles = item.TotalFiles
		}
	}
	if documentFiles != 2 || imageFiles != 1 {
		t.Fatalf("category stats = %#v", categoryStats)
	}
	driveStats, err := svc.DriveStatistics(ctx, 1001)
	if err != nil || driveStats.TotalFiles != 3 || driveStats.TotalFolders != 3 || driveStats.TotalBytes != 2300 {
		t.Fatalf("DriveStatistics() = %#v, %v", driveStats, err)
	}

	moved, err := svc.BulkMove(ctx, 1001, []uuid.UUID{pdfID}, &archiveID, "rename")
	if err != nil || len(moved) != 1 || moved[0].Name != "Annual Report (1).pdf" {
		t.Fatalf("BulkMove(rename) = %#v, %v", moved, err)
	}
	trashed, err := svc.BulkTrash(ctx, 1001, []uuid.UUID{docsID})
	if err != nil || len(trashed) != 3 {
		t.Fatalf("BulkTrash() = %#v, %v", trashed, err)
	}
	for _, id := range []uuid.UUID{docsID, reportsID, imageID} {
		file, err := svc.Get(ctx, 1001, id)
		if err != nil || file.Status != sqlcgen.FileStatusTrashed {
			t.Fatalf("trashed subtree file %s = %#v, %v", id, file, err)
		}
	}
}

func TestCatalogRejectsInvalidParent(t *testing.T) {
	db := testpostgres.New(t)
	ctx := context.Background()
	seedUser(t, db.Pool, 1001)
	svc := catalog.NewService(db.Pool, nil)
	missing := uuid.New()
	if _, err := svc.CreateFolder(ctx, catalog.CreateFolderInput{UserID: 1001, ParentID: &missing, Name: "child"}); !errors.Is(err, catalog.ErrInvalidParent) {
		t.Fatalf("invalid parent error = %v", err)
	}
}

func seedUser(t testing.TB, db *pgxpool.Pool, userID int64) {
	t.Helper()
	if _, err := db.Exec(context.Background(), "INSERT INTO users (user_id) VALUES ($1)", userID); err != nil {
		t.Fatalf("seed user %d: %v", userID, err)
	}
}

func mustUUID(t testing.TB, value pgtype.UUID) uuid.UUID {
	t.Helper()
	id, ok := dbtypes.GoogleUUID(value)
	if !ok {
		t.Fatal("expected UUID value")
	}
	return id
}

func seedFile(t testing.TB, db *pgxpool.Pool, userID int64, parentID *uuid.UUID, name, mime string, size int64, updatedAt time.Time) uuid.UUID {
	t.Helper()
	display, normalized, err := catalog.NormalizeName(name)
	if err != nil {
		t.Fatalf("normalize seed file: %v", err)
	}
	id := uuid.New()
	if _, err := db.Exec(context.Background(), `
INSERT INTO files (
  id, user_id, parent_id, name, normalized_name, kind, mime_type, size,
  encryption, status, mod_time, created_at, updated_at
) VALUES ($1,$2,$3,$4,$5,'file',$6,$7,false,'active',$8,$8,$8)`,
		id, userID, parentID, display, normalized, mime, size, updatedAt.UTC()); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	return id
}
