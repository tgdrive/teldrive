//go:build integration

package api

import (
	"context"
	"testing"

	"github.com/tgdrive/teldrive/v2/internal/api/gen"
	"github.com/tgdrive/teldrive/v2/internal/channels"
	"github.com/tgdrive/teldrive/v2/internal/principal"
	"github.com/tgdrive/teldrive/v2/internal/telegramstore"
	testpostgres "github.com/tgdrive/teldrive/v2/internal/testutil/postgres"
)

type channelPickerAccount struct{}

func (channelPickerAccount) DiscoverChannels(context.Context, int64) ([]telegramstore.DiscoveredChannel, error) {
	return []telegramstore.DiscoveredChannel{{ID: 9001, Name: "chosen"}, {ID: 9002, Name: "unchecked"}}, nil
}

func (channelPickerAccount) ProfilePhoto(context.Context, int64) (telegramstore.ProfilePhoto, bool, error) {
	return telegramstore.ProfilePhoto{}, false, nil
}

func TestSyncChannelsRegistersOnlyChosenChannels(t *testing.T) {
	db := testpostgres.New(t)
	ctx := context.Background()
	if _, err := db.Pool.Exec(ctx, "INSERT INTO users (user_id) VALUES (1001); INSERT INTO channels (user_id, channel_id, name, selected) VALUES (1001, 9003, 'existing', true)"); err != nil {
		t.Fatal(err)
	}
	h := &Handler{Channels: channels.NewService(db.Pool, nil, channels.Config{}), TelegramAccount: channelPickerAccount{}}
	req := &gen.ChannelSyncRequest{ChannelIds: []int64{9001}}
	if _, err := h.SyncChannels(ctx, req, gen.SyncChannelsParams{}); err == nil {
		t.Fatal("unauthenticated sync accepted")
	}
	ctx = principal.WithIdentity(ctx, principal.Identity{UserID: 1001})
	if _, err := h.SyncChannels(ctx, &gen.ChannelSyncRequest{ChannelIds: []int64{9001, 9999}}, gen.SyncChannelsParams{}); err == nil {
		t.Fatal("undiscovered channel accepted")
	}
	var count int
	if err := db.Pool.QueryRow(ctx, "SELECT count(*) FROM channels").Scan(&count); err != nil || count != 1 {
		t.Fatalf("invalid request modified channels: count=%d, err=%v", count, err)
	}
	if _, err := h.SyncChannels(ctx, req, gen.SyncChannelsParams{}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.SyncChannels(ctx, &gen.ChannelSyncRequest{ChannelIds: []int64{}}, gen.SyncChannelsParams{}); err != nil {
		t.Fatal(err)
	}
	rows, err := h.Channels.List(ctx, channels.ListInput{UserID: 1001})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("registered channels = %d, want 2", len(rows))
	}
	for _, row := range rows {
		if row.ChannelID == 9002 {
			t.Fatal("unchecked channel registered")
		}
		if row.ChannelID == 9003 && !row.Selected {
			t.Fatal("active upload selection changed")
		}
	}
}
