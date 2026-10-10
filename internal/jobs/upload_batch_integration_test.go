//go:build integration

package jobs

import (
	"context"
	"testing"

	"github.com/tgdrive/teldrive/v2/internal/catalog"
	testpostgres "github.com/tgdrive/teldrive/v2/internal/testutil/postgres"
)

func TestUploadBatchWorkerCreatesDestinationPath(t *testing.T) {
	db := testpostgres.New(t)
	ctx := context.Background()
	if _, err := db.Pool.Exec(ctx, "INSERT INTO users (user_id) VALUES (1001)"); err != nil {
		t.Fatal(err)
	}
	catalogService := catalog.NewService(db.Pool, nil)
	worker := NewUploadBatchWorker(nil, catalogService)

	parentID, err := worker.resolveDestination(ctx, UploadBatchArgs{UserID: 1001, Destination: "/videos/new"})
	if err != nil {
		t.Fatalf("resolveDestination() error = %v", err)
	}
	resolved, err := catalogService.ResolveFolderPath(ctx, 1001, nil, "/videos/new")
	if err != nil {
		t.Fatalf("ResolveFolderPath() error = %v", err)
	}
	if resolved == nil || resolved.String() != parentID {
		t.Fatalf("resolved destination = %v, worker parent ID = %q", resolved, parentID)
	}
}
