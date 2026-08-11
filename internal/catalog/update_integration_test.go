//go:build integration

package catalog_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/tgdrive/teldrive/v2/internal/catalog"
	testpostgres "github.com/tgdrive/teldrive/v2/internal/testutil/postgres"
)

func TestUpdateMetadataAgainstRealPostgres(t *testing.T) {
	db := testpostgres.New(t)
	ctx := context.Background()
	seedUser(t, db.Pool, 1001)
	svc := catalog.NewService(db.Pool)
	created, err := svc.CreateFolder(ctx, catalog.CreateFolderInput{UserID: 1001, Name: "Before"})
	if err != nil {
		t.Fatal(err)
	}
	fileID := mustUUID(t, created.ID)
	name := "After"
	modTime := time.Date(2026, 7, 1, 10, 30, 0, 0, time.UTC)
	updated, err := svc.Update(ctx, catalog.UpdateInput{
		UserID: 1001, FileID: fileID, ExpectedGeneration: &created.Generation,
		Name: &name, ModTime: &modTime,
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if updated.Name != name || updated.NormalizedName != "After" || !updated.ModTime.Time.Equal(modTime) || updated.Generation != created.Generation+1 {
		t.Fatalf("updated file = %#v", updated)
	}
	if _, err := svc.Update(ctx, catalog.UpdateInput{
		UserID: 1001, FileID: fileID, ExpectedGeneration: &created.Generation, Name: &name,
	}); !errors.Is(err, catalog.ErrPrecondition) {
		t.Fatalf("stale Update() error = %v", err)
	}
	if _, err := svc.Update(ctx, catalog.UpdateInput{UserID: 1001, FileID: uuid.New()}); !errors.Is(err, catalog.ErrInvalidName) {
		t.Fatalf("empty Update() error = %v", err)
	}
}
