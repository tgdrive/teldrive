package api

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/tgdrive/teldrive/v2/internal/api/gen"
)

// Detailed transport identities are visible only to the file owner, never through a public ticket.
func (h *Handler) GetFilePlaybackStatistics(ctx context.Context, params gen.GetFilePlaybackStatisticsParams) (gen.GetFilePlaybackStatisticsRes, error) {
	actor, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, mapServiceError(err)
	}
	if h.Catalog == nil || h.Downloader == nil {
		return nil, mapServiceError(ErrOperationUnavailable)
	}
	id := googleUUID(params.FileId)
	file, err := h.Catalog.Get(ctx, actor, id)
	if err != nil {
		return nil, mapServiceError(err)
	}
	now := time.Now()
	snapshot := h.Downloader.Statistics(actor, id)
	out := &gen.PlaybackStatistics{Scope: gen.PlaybackStatisticsScopeFile, FileId: params.FileId, Name: file.Name, MimeType: file.MimeType.String, Size: file.Size.Int64, Encrypted: file.Encryption, SampledAt: now, ActiveStreams: int32(snapshot.Active), Accounts: []gen.StreamAccountStatistics{}}
	if !snapshot.Started.IsZero() {
		out.StartedAt = gen.NewOptDateTime(snapshot.Started)
	}
	for _, a := range snapshot.Accounts {
		row := gen.StreamAccountStatistics{ID: a.ID, Name: "Cuenta de Telegram", Kind: gen.StreamAccountStatisticsKindUser, Bytes: a.Bytes, BytesPerSecond: a.BytesPerSecond(now), Requests: a.Requests, Errors: a.Errors, ActiveRequests: int32(a.Active), LatencyMs: a.LatencyMS, ChannelId: a.ChannelID, MessageId: a.MessageID, Offset: a.Offset}
		if !a.LastSeen.IsZero() {
			row.LastSeen = gen.NewOptDateTime(a.LastSeen)
		}
		if h.Bots != nil {
			if bot, e := h.Bots.Get(ctx, actor, a.ID); e == nil {
				row.Name = bot.Name.String
				row.Alias = bot.Username.String
				row.Kind = gen.StreamAccountStatisticsKindBot
			}
		}
		out.Accounts = append(out.Accounts, row)
		out.Bytes += row.Bytes
		out.BytesPerSecond += row.BytesPerSecond
	}
	sort.SliceStable(out.Accounts, func(i, j int) bool {
		a, b := out.Accounts[i], out.Accounts[j]
		if strings.EqualFold(a.Name, b.Name) {
			return a.ID < b.ID
		}
		return strings.ToLower(a.Name) < strings.ToLower(b.Name)
	})
	return out, nil
}
