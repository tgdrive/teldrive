package jobs

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/tgdrive/teldrive/v2/internal/db/sqlcgen"
	"github.com/tgdrive/teldrive/v2/internal/dbtypes"
	"github.com/tgdrive/teldrive/v2/internal/telegramstore"
)

const OrphanCleanupKind = "teldrive_cleanup_orphaned_telegram_parts"

// maxBrokenFiles caps the broken-file list kept in the job output; the
// brokenTotal counter always reflects the full count. River rejects job
// output above 32MB, and names make entries unbounded.
const maxBrokenFiles = 500

type OrphanCleanupArgs struct{}

func (OrphanCleanupArgs) Kind() string { return OrphanCleanupKind }
func (OrphanCleanupArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: CleanupQueue, MaxAttempts: 3, Priority: 3}
}

// OrphanCleanupOutput is recorded as the River job output when the sweep
// completes, so operators can inspect per-run counters without log access.
type OrphanCleanupOutput struct {
	Channels    int       `json:"channels"`
	Scanned     int       `json:"scanned"`
	Deleted     int       `json:"deleted"`
	Cutoff      time.Time `json:"cutoff"`
	CompletedAt time.Time `json:"completedAt"`
	// BrokenFiles lists active files with DB-referenced messages missing from
	// Telegram, so owners know what to re-upload. Capped at maxBrokenFiles;
	// BrokenTotal always holds the full count.
	BrokenFiles     []BrokenFile `json:"brokenFiles"`
	BrokenTotal     int          `json:"brokenTotal"`
	BrokenTruncated bool         `json:"brokenTruncated"`
}

// BrokenFile is one active file with at least one referenced part message
// absent from its Telegram channel.
type BrokenFile struct {
	FileID            string  `json:"fileId"`
	Name              string  `json:"name"`
	Size              int64   `json:"size"`
	ChannelID         int64   `json:"channelId"`
	MissingMessageIDs []int64 `json:"missingMessageIds"`
}

// findBrokenFiles returns files with referenced messages absent from the
// Telegram listing, grouped by file and sorted by name. The returned list is
// capped at limit entries; total is the uncapped file count.
func findBrokenFiles(seen map[int64]struct{}, rows []*sqlcgen.ListChannelReferencedPartsRow, channelID int64, limit int) (broken []BrokenFile, total int) {
	type pending struct {
		name string
		size int64
		ids  []int64
	}
	byFile := make(map[string]*pending)
	order := make([]string, 0)
	for _, row := range rows {
		if _, ok := seen[row.MessageID]; ok {
			continue
		}
		fileID, ok := dbtypes.GoogleUUID(row.FileID)
		if !ok {
			continue
		}
		key := fileID.String()
		entry, ok := byFile[key]
		if !ok {
			entry = &pending{name: row.FileName, size: row.FileSize.Int64}
			byFile[key] = entry
			order = append(order, key)
		}
		entry.ids = append(entry.ids, row.MessageID)
	}
	files := make([]BrokenFile, 0, len(order))
	for _, key := range order {
		entry := byFile[key]
		slices.Sort(entry.ids)
		files = append(files, BrokenFile{FileID: key, Name: entry.name, Size: entry.size, ChannelID: channelID, MissingMessageIDs: entry.ids})
	}
	slices.SortFunc(files, func(a, b BrokenFile) int {
		if result := strings.Compare(a.Name, b.Name); result != 0 {
			return result
		}
		return strings.Compare(a.FileID, b.FileID)
	})
	total = len(files)
	if limit < 0 {
		limit = 0
	}
	if len(files) > limit {
		files = files[:limit]
	}
	return files, total
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
	return 4 * time.Hour
}

func (w *OrphanedTelegramPartsCleanupWorker) Work(ctx context.Context, job *river.Job[OrphanCleanupArgs]) error {
	channels, err := w.queries.ListChannelsForOrphanCleanup(ctx)
	if err != nil {
		return fmt.Errorf("list channels for orphan cleanup: %w", err)
	}
	cutoff := time.Now().UTC().Add(-w.minimumAge)
	var scanned, deleted, brokenTotal int
	brokenFiles := make([]BrokenFile, 0)
	for _, channel := range channels {
		channelScanned, channelDeleted, channelBroken := 0, 0, 0
		seen := make(map[int64]struct{})
		beforeID := int64(0)
		for {
			page, err := w.lister.ListDocumentMessages(ctx, telegramstore.ListDocumentMessagesRequest{
				UserID: channel.UserID, ChannelID: channel.ChannelID, BeforeID: beforeID, Limit: 100,
			})
			if err != nil {
				return fmt.Errorf("list Telegram documents for channel %d: %w", channel.ChannelID, err)
			}
			scanned += len(page.Messages)
			channelScanned += len(page.Messages)
			for _, message := range page.Messages {
				seen[message.ID] = struct{}{}
			}
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
					channelDeleted += len(orphans)
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
		referenced, err := w.queries.ListChannelReferencedParts(ctx, sqlcgen.ListChannelReferencedPartsParams{
			TargetChannelID: channel.ChannelID, TargetUserID: channel.UserID,
		})
		if err != nil {
			return fmt.Errorf("list referenced parts for channel %d: %w", channel.ChannelID, err)
		}
		channelBrokenFiles, channelBrokenTotal := findBrokenFiles(seen, referenced, channel.ChannelID, maxBrokenFiles-len(brokenFiles))
		brokenFiles = append(brokenFiles, channelBrokenFiles...)
		brokenTotal += channelBrokenTotal
		channelBroken = channelBrokenTotal
		slog.DebugContext(ctx, "orphaned Telegram part cleanup: channel completed",
			"user_id", channel.UserID, "channel_id", channel.ChannelID,
			"scanned", channelScanned, "deleted", channelDeleted, "broken", channelBroken)
	}
	slog.InfoContext(ctx, "orphaned Telegram part cleanup completed", "channels", len(channels), "scanned", scanned, "deleted", deleted, "broken", brokenTotal, "cutoff", cutoff)
	output := OrphanCleanupOutput{Channels: len(channels), Scanned: scanned, Deleted: deleted, Cutoff: cutoff, CompletedAt: time.Now().UTC(),
		BrokenFiles: brokenFiles, BrokenTotal: brokenTotal, BrokenTruncated: brokenTotal > len(brokenFiles)}
	if job.JobRow != nil {
		return river.RecordOutput(ctx, output)
	}
	return nil
}
