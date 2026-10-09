package channels

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/tgdrive/teldrive/v2/internal/db/sqlcgen"
)

// Sync upserts Telegram channels the authenticated user can administer. It
// deliberately does not delete missing rows: a temporarily hidden or
// inaccessible Telegram dialog must not invalidate stored objects.
func (s *Service) Sync(ctx context.Context, userID int64, remote []RemoteChannel) ([]*sqlcgen.Channel, error) {
	if s == nil || s.pool == nil || userID <= 0 {
		return nil, ErrInvalidOwner
	}
	unique := make(map[int64]RemoteChannel, len(remote))
	for _, channel := range remote {
		channel.Name = strings.TrimSpace(channel.Name)
		if channel.ID == 0 || channel.Name == "" {
			return nil, ErrInvalidChannel
		}
		unique[channel.ID] = channel
	}
	items := slices.SortedFunc(maps.Values(unique), func(a, b RemoteChannel) int {
		return cmp.Or(cmp.Compare(a.Name, b.Name), cmp.Compare(a.ID, b.ID))
	})

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin channel sync: %w", err)
	}
	defer tx.Rollback(ctx)
	queries := s.queries.WithTx(tx)
	encoded, err := json.Marshal(items)
	if err != nil {
		return nil, fmt.Errorf("encode discovered channels: %w", err)
	}
	inserted, err := queries.UpsertDiscoveredChannels(ctx, sqlcgen.UpsertDiscoveredChannelsParams{
		UserID: userID, Channels: encoded,
	})
	if err != nil {
		return nil, fmt.Errorf("upsert discovered channels: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit channel sync: %w", err)
	}
	byID := make(map[int64]*sqlcgen.Channel, len(inserted))
	for _, row := range inserted {
		byID[row.ChannelID] = row
	}
	rows := make([]*sqlcgen.Channel, 0, len(items))
	for _, item := range items {
		row := byID[item.ID]
		if row == nil {
			return nil, ErrInvalidChannel
		}
		rows = append(rows, row)
	}
	return rows, nil
}
