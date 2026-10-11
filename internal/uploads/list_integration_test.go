//go:build integration

package uploads_test

import (
	"context"
	"testing"

	"github.com/tgdrive/teldrive/v2/internal/db/sqlcgen"
	"github.com/tgdrive/teldrive/v2/internal/dbtypes"
	testpostgres "github.com/tgdrive/teldrive/v2/internal/testutil/postgres"
	"github.com/tgdrive/teldrive/v2/internal/uploads"
)

func TestListUploadsAndPartsAgainstRealPostgres(t *testing.T) {
	db := testpostgres.New(t)
	ctx := context.Background()
	seedUploadOwner(t, db.Pool, 1001, 9001)
	svc := uploads.NewService(db.Pool)

	first, err := svc.Create(ctx, uploads.CreateInput{UserID: 1001, Name: "first.bin", ExpectedSize: 1, PartSize: 1})
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.Create(ctx, uploads.CreateInput{UserID: 1001, Name: "second.bin", ExpectedSize: 1, PartSize: 1})
	if err != nil {
		t.Fatal(err)
	}
	secondID, _ := dbtypes.GoogleUUID(second.ID)
	if _, err := svc.Abort(ctx, 1001, secondID); err != nil {
		t.Fatal(err)
	}

	openState := sqlcgen.UploadStateOpen
	open, err := svc.List(ctx, uploads.ListInput{UserID: 1001, State: &openState, Limit: 10})
	if err != nil {
		t.Fatalf("List(open) error = %v", err)
	}
	if len(open) != 1 || open[0].Name != first.Name {
		t.Fatalf("open uploads = %#v", open)
	}
	all, err := svc.List(ctx, uploads.ListInput{UserID: 1001, Limit: 1})
	if err != nil {
		t.Fatalf("List(all) error = %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("first page size = %d", len(all))
	}
	cursorID, _ := dbtypes.GoogleUUID(all[0].ID)
	cursorTime := all[0].CreatedAt.Time
	next, err := svc.List(ctx, uploads.ListInput{
		UserID: 1001, AfterID: &cursorID, AfterCreatedAt: &cursorTime, Limit: 10,
	})
	if err != nil {
		t.Fatalf("List(next) error = %v", err)
	}
	if len(next) != 1 {
		t.Fatalf("next page size = %d", len(next))
	}

	firstID, _ := dbtypes.GoogleUUID(first.ID)
	blockHashes, checksum := hashMetadata([]byte("x"))
	claim, err := svc.ClaimPart(ctx, uploads.ClaimPartInput{
		UserID: 1001, UploadID: firstID, PartNo: 1, ChannelID: 9001, PlainSize: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.StorePart(ctx, uploads.StorePartInput{
		UploadID: firstID, PartNo: 1, LeaseToken: claim.LeaseToken,
		MessageID: 99, StoredSize: 1, Checksum: checksum, BlockHashes: blockHashes,
	}); err != nil {
		t.Fatal(err)
	}
	parts, err := svc.ListParts(ctx, uploads.ListPartsInput{UserID: 1001, UploadID: firstID, Limit: 10})
	if err != nil {
		t.Fatalf("ListParts() error = %v", err)
	}
	if len(parts) != 1 || parts[0].MessageID.Int64 != 99 {
		t.Fatalf("parts = %#v", parts)
	}
	after := int32(1)
	empty, err := svc.ListParts(ctx, uploads.ListPartsInput{
		UserID: 1001, UploadID: firstID, AfterPartNo: &after, Limit: 10,
	})
	if err != nil || len(empty) != 0 {
		t.Fatalf("ListParts(after) = %#v, %v", empty, err)
	}
}
