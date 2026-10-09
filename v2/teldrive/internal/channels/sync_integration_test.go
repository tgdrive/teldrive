//go:build integration

package channels_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tgdrive/teldrive/v2/internal/channels"
	testpostgres "github.com/tgdrive/teldrive/v2/internal/testutil/postgres"
	"github.com/tgdrive/teldrive/v2/internal/testutil/querytrace"
)

func TestSyncUsesOneUpsertForWideDiscovery(t *testing.T) {
	db := testpostgres.New(t)
	ctx := context.Background()
	seedChannelOwner(t, db.Pool, 1001)
	remote := make([]channels.RemoteChannel, 500)
	for index := range remote {
		remote[index] = channels.RemoteChannel{ID: int64(10_000 + index), Name: fmt.Sprintf("channel-%03d", index)}
	}
	tracer := &querytrace.Counter{}
	config, err := pgxpool.ParseConfig(db.URL)
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.Tracer = tracer
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	rows, err := channels.NewService(pool, nil, channels.Config{}).Sync(ctx, 1001, remote)
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	if len(rows) != len(remote) {
		t.Fatalf("synced channels = %d, want %d", len(rows), len(remote))
	}
	if got := tracer.Count("UpsertDiscoveredChannels"); got != 1 {
		t.Fatalf("UpsertDiscoveredChannels queries = %d, want 1", got)
	}
	if got := tracer.Count("UpsertDiscoveredChannel"); got != 0 {
		t.Fatalf("UpsertDiscoveredChannel queries = %d, want 0", got)
	}
}

func TestSyncDeduplicatesSortsAndUpdatesChannels(t *testing.T) {
	db := testpostgres.New(t)
	ctx := context.Background()
	seedChannelOwner(t, db.Pool, 1001)
	svc := channels.NewService(db.Pool, nil, channels.Config{})

	rows, err := svc.Sync(ctx, 1001, []channels.RemoteChannel{
		{ID: 9002, Name: "  beta  "},
		{ID: 9001, Name: "alpha"},
		{ID: 9002, Name: "beta renamed"},
		{ID: 9003, Name: "alpha"},
	})
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("Sync() rows = %d, want 3", len(rows))
	}
	if rows[0].Name != "alpha" || rows[0].ChannelID != 9001 || rows[1].ChannelID != 9003 || rows[2].Name != "beta renamed" {
		t.Fatalf("Sync() ordering = %#v", rows)
	}

	updated, err := svc.Sync(ctx, 1001, []channels.RemoteChannel{{ID: 9001, Name: "alpha updated"}})
	if err != nil {
		t.Fatalf("second Sync() error = %v", err)
	}
	if len(updated) != 1 || updated[0].Name != "alpha updated" {
		t.Fatalf("updated rows = %#v", updated)
	}

	if _, err := svc.Sync(ctx, 0, nil); !errors.Is(err, channels.ErrInvalidOwner) {
		t.Fatalf("invalid owner error = %v", err)
	}
	if _, err := svc.Sync(ctx, 1001, []channels.RemoteChannel{{ID: 0, Name: "bad"}}); !errors.Is(err, channels.ErrInvalidChannel) {
		t.Fatalf("invalid channel ID error = %v", err)
	}
	if _, err := svc.Sync(ctx, 1001, []channels.RemoteChannel{{ID: 1, Name: "  "}}); !errors.Is(err, channels.ErrInvalidChannel) {
		t.Fatalf("blank channel name error = %v", err)
	}
}
