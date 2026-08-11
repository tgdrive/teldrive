package jobs

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/tgdrive/teldrive/v2/internal/db/sqlcgen"
	"github.com/tgdrive/teldrive/v2/internal/telegramstore"
)

const OrphanCleanupKind = "teldrive_cleanup_orphaned_telegram_parts"

type OrphanCleanupArgs struct {
	PageSize int32 `json:"page_size,omitempty"`
}

func (OrphanCleanupArgs) Kind() string { return OrphanCleanupKind }

func (OrphanCleanupArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: CleanupQueue, MaxAttempts: 3, Priority: 3}
}

type OrphanedTelegramPartsCleanupWorker struct {
	river.WorkerDefaults[OrphanCleanupArgs]
	queries    *sqlcgen.Queries
	lister     telegramstore.DocumentMessageLister
	storage    telegramstore.Storage
	minimumAge time.Duration
}

func NewOrphanedTelegramPartsCleanupWorker(pool *pgxpool.Pool, storage telegramstore.Storage, lister telegramstore.DocumentMessageLister, minimumAge time.Duration) *OrphanedTelegramPartsCleanupWorker {
	return &OrphanedTelegramPartsCleanupWorker{queries: sqlcgen.New(pool), storage: storage, lister: lister, minimumAge: minimumAge}
}

func (w *OrphanedTelegramPartsCleanupWorker) Timeout(*river.Job[OrphanCleanupArgs]) time.Duration {
	return 2 * time.Hour
}

func (w *OrphanedTelegramPartsCleanupWorker) Work(ctx context.Context, job *river.Job[OrphanCleanupArgs]) error {
	pageSize := int(job.Args.PageSize)
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 100
	}
	channels, err := w.queries.ListChannelsForOrphanCleanup(ctx)
	if err != nil {
		return fmt.Errorf("list channels for orphan cleanup: %w", err)
	}
	cutoff := time.Now().UTC().Add(-w.minimumAge)
	var scanned, deleted int
	for _, channel := range channels {
		beforeID := int64(0)
		for {
			page, err := w.lister.ListDocumentMessages(ctx, telegramstore.ListDocumentMessagesRequest{
				UserID: channel.UserID, ChannelID: channel.ChannelID, BeforeID: beforeID, Limit: pageSize,
			})
			if err != nil {
				return fmt.Errorf("list Telegram documents for channel %d: %w", channel.ChannelID, err)
			}
			scanned += len(page.Messages)
			candidateIDs := make([]int64, 0, len(page.Messages))
			for _, message := range page.Messages {
				if message.CreatedAt.Before(cutoff) {
					candidateIDs = append(candidateIDs, message.ID)
				}
			}
			if len(candidateIDs) > 0 {
				referenced, err := w.queries.ListReferencedMessageIDs(ctx, sqlcgen.ListReferencedMessageIDsParams{
					TargetChannelID: channel.ChannelID, MessageIds: candidateIDs,
				})
				if err != nil {
					return fmt.Errorf("list referenced messages for channel %d: %w", channel.ChannelID, err)
				}
				refs := make(map[int64]struct{}, len(referenced))
				for _, id := range referenced {
					refs[id] = struct{}{}
				}
				orphans := candidateIDs[:0]
				for _, id := range candidateIDs {
					if _, ok := refs[id]; !ok {
						orphans = append(orphans, id)
					}
				}
				if len(orphans) > 0 {
					if err := w.storage.DeleteMessages(ctx, channel.UserID, channel.ChannelID, orphans); err != nil {
						return fmt.Errorf("delete orphaned Telegram documents from channel %d: %w", channel.ChannelID, err)
					}
					deleted += len(orphans)
				}
			}
			if page.Exhausted {
				break
			}
			if page.BeforeID <= 0 || page.BeforeID == beforeID {
				return fmt.Errorf("list Telegram documents for channel %d: pagination did not advance", channel.ChannelID)
			}
			beforeID = page.BeforeID
		}
	}
	slog.InfoContext(ctx, "orphaned Telegram part cleanup completed", "channels", len(channels), "scanned", scanned, "deleted", deleted, "cutoff", cutoff)
	return nil
}
