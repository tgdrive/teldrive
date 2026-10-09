//go:build integration

package channels_test

import (
	"context"
	"errors"
	"testing"

	"github.com/tgdrive/teldrive/v2/internal/channels"
	testpostgres "github.com/tgdrive/teldrive/v2/internal/testutil/postgres"
)

func TestChannelAdminLifecycleAgainstRealPostgres(t *testing.T) {
	db := testpostgres.New(t)
	seedChannelOwner(t, db.Pool, 1001)
	creator := &fakeCreator{nextID: 9100}
	service := channels.NewService(db.Pool, creator, channels.Config{PartLimit: 100, AutoCreate: true, NamePrefix: "storage"})
	ctx := context.Background()

	first, err := service.Create(ctx, 1001, "first", true)
	if err != nil {
		t.Fatalf("Create(first) error = %v", err)
	}
	second, err := service.Create(ctx, 1001, "second", false)
	if err != nil {
		t.Fatalf("Create(second) error = %v", err)
	}
	if !first.Selected || second.Selected {
		t.Fatalf("created selections = first %v second %v", first.Selected, second.Selected)
	}
	rows, err := service.List(ctx, channels.ListInput{UserID: 1001, Limit: 10})
	if err != nil || len(rows) != 2 {
		t.Fatalf("List() = %#v, %v", rows, err)
	}
	page, err := service.List(ctx, channels.ListInput{UserID: 1001, Limit: 1})
	if err != nil || len(page) != 1 {
		t.Fatalf("List(page) = %#v, %v", page, err)
	}
	afterCreatedAt := page[0].CreatedAt.Time
	afterChannelID := page[0].ChannelID
	nextPage, err := service.List(ctx, channels.ListInput{
		UserID: 1001, AfterCreatedAt: &afterCreatedAt, AfterChannelID: &afterChannelID, Limit: 10,
	})
	if err != nil || len(nextPage) != 1 {
		t.Fatalf("List(next page) = %#v, %v", nextPage, err)
	}
	selected, err := service.Select(ctx, 1001, second.ChannelID)
	if err != nil || !selected.Selected {
		t.Fatalf("Select() = %#v, %v", selected, err)
	}
	if err := service.Delete(ctx, 1001, first.ChannelID); err != nil {
		t.Fatalf("Delete(first) error = %v", err)
	}
	if creator.deleteCalls() != 1 {
		t.Fatalf("remote deletes = %d", creator.deleteCalls())
	}
	if err := service.Delete(ctx, 1001, second.ChannelID); !errors.Is(err, channels.ErrSelectedChannel) {
		t.Fatalf("Delete(selected) error = %v", err)
	}
	third, err := service.Create(ctx, 1001, "referenced", false)
	if err != nil {
		t.Fatalf("Create(third) error = %v", err)
	}
	insertStoredPart(t, db.Pool, 1001, third.ChannelID, 123)
	if err := service.Delete(ctx, 1001, third.ChannelID); !errors.Is(err, channels.ErrChannelInUse) {
		t.Fatalf("Delete(referenced) error = %v", err)
	}
	unnamed, err := service.Create(ctx, 1001, "", false)
	if err != nil || unnamed.Name == "" {
		t.Fatalf("Create(default name) = %#v, %v", unnamed, err)
	}

	insertChannel(t, db.Pool, 1001, 9999, false)
	conflictingCreator := &fakeCreator{fixedID: 9999}
	conflictingService := channels.NewService(db.Pool, conflictingCreator, channels.Config{PartLimit: 100, AutoCreate: true, NamePrefix: "storage"})
	if _, err := conflictingService.Create(ctx, 1001, "conflict", false); err == nil {
		t.Fatal("expected create conflict")
	}
	if conflictingCreator.deleteCalls() != 1 {
		t.Fatalf("create compensation deletes = %d", conflictingCreator.deleteCalls())
	}
}
