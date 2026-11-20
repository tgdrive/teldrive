package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/tgdrive/teldrive/v2/internal/contentcrypto"
	"github.com/tgdrive/teldrive/v2/internal/database"
	"github.com/tgdrive/teldrive/v2/internal/telegramstore"
)

type partToBackfill struct {
	FileID     uuid.UUID
	PartNo     int32
	ChannelID  int64
	MessageID  int64
	UserID     int64
	Encryption bool
}

// BackfillPartSizes resolves missing plain_size and stored_size for file parts from Telegram
// and updates them in the database in batches.
func (a *App) BackfillPartSizes(ctx context.Context, batchSize int, delay time.Duration, limit int64) error {
	if batchSize <= 0 || batchSize > 100 {
		batchSize = 100
	}
	if delay < 0 {
		delay = time.Second
	}

	schema := a.config.Database.Schema
	if schema == "" {
		schema = database.DefaultSchema
	}

	var totalMissing int64
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM %s.file_parts WHERE plain_size IS NULL OR stored_size IS NULL", schema)
	if err := a.pool.QueryRow(ctx, countQuery).Scan(&totalMissing); err != nil {
		return fmt.Errorf("count parts without sizes: %w", err)
	}

	if totalMissing == 0 {
		slog.Info("backfill.completed", "status", "no parts missing sizes")
		fmt.Println("All file parts already have sizes recorded. Nothing to backfill.")
		return nil
	}

	targetCount := totalMissing
	if limit > 0 && limit < targetCount {
		targetCount = limit
	}
	slog.Info("backfill.started", "total_missing", totalMissing, "target", targetCount, "batch_size", batchSize, "delay", delay)
	fmt.Printf("Starting part sizes backfill: %d parts missing sizes (target: %d)\n", totalMissing, targetCount)

	startTime := time.Now()
	var processedTotal int64
	var updatedTotal int64
	var skippedTotal int64

	var lastUserID int64
	var lastChannelID int64
	var lastFileID uuid.UUID
	var lastPartNo int32
	hasCursor := false

	var currentSession telegramstore.DownloadSession
	var currentSessionUserID int64
	defer func() {
		if currentSession != nil {
			_ = currentSession.Close()
		}
	}()

	openUserSession := func(userID int64) (telegramstore.DownloadSession, error) {
		if currentSession != nil && currentSessionUserID == userID {
			return currentSession, nil
		}
		if currentSession != nil {
			_ = currentSession.Close()
			currentSession = nil
			currentSessionUserID = 0
		}
		if opener, ok := a.storage.(telegramstore.DownloadSessionOpener); ok {
			session, err := opener.OpenDownloadSession(ctx, userID)
			if err != nil {
				return nil, err
			}
			currentSession = session
			currentSessionUserID = userID
			return session, nil
		}
		return nil, errors.New("storage does not support download sessions")
	}

	for {
		if ctx.Err() != nil {
			break
		}
		if limit > 0 && processedTotal >= limit {
			break
		}

		fetchLimit := int64(batchSize * 5)
		if limit > 0 && (limit-processedTotal) < fetchLimit {
			fetchLimit = limit - processedTotal
		}

		var query string
		var rowsArgs []any
		if !hasCursor {
			query = fmt.Sprintf(`
				SELECT part.file_id, part.part_no, part.channel_id, part.message_id, f.user_id, f.encryption
				FROM %s.file_parts AS part
				JOIN %s.files AS f ON f.id = part.file_id
				WHERE (part.plain_size IS NULL OR part.stored_size IS NULL)
				ORDER BY f.user_id, part.channel_id, part.file_id, part.part_no
				LIMIT $1`, schema, schema)
			rowsArgs = []any{fetchLimit}
		} else {
			query = fmt.Sprintf(`
				SELECT part.file_id, part.part_no, part.channel_id, part.message_id, f.user_id, f.encryption
				FROM %s.file_parts AS part
				JOIN %s.files AS f ON f.id = part.file_id
				WHERE (part.plain_size IS NULL OR part.stored_size IS NULL)
				  AND (f.user_id, part.channel_id, part.file_id, part.part_no) > ($1, $2, $3, $4)
				ORDER BY f.user_id, part.channel_id, part.file_id, part.part_no
				LIMIT $5`, schema, schema)
			rowsArgs = []any{lastUserID, lastChannelID, lastFileID, lastPartNo, fetchLimit}
		}

		rows, err := a.pool.Query(ctx, query, rowsArgs...)
		if err != nil {
			return fmt.Errorf("fetch parts without sizes: %w", err)
		}

		var batch []partToBackfill
		for rows.Next() {
			var p partToBackfill
			if err := rows.Scan(&p.FileID, &p.PartNo, &p.ChannelID, &p.MessageID, &p.UserID, &p.Encryption); err != nil {
				rows.Close()
				return fmt.Errorf("scan part row: %w", err)
			}
			batch = append(batch, p)
		}
		rows.Close()

		if len(batch) == 0 {
			break
		}

		last := batch[len(batch)-1]
		lastUserID = last.UserID
		lastChannelID = last.ChannelID
		lastFileID = last.FileID
		lastPartNo = last.PartNo
		hasCursor = true

		i := 0
		for i < len(batch) {
			if ctx.Err() != nil {
				break
			}
			chunkStart := i
			userID := batch[chunkStart].UserID
			channelID := batch[chunkStart].ChannelID

			for i < len(batch) && (i-chunkStart) < batchSize && batch[i].UserID == userID && batch[i].ChannelID == channelID {
				i++
			}
			chunk := batch[chunkStart:i]
			processedTotal += int64(len(chunk))

			session, err := openUserSession(userID)
			if err != nil {
				slog.Error("backfill.session_failed", "user_id", userID, "error", err)
				skippedTotal += int64(len(chunk))
				continue
			}

			messageIDs := make([]int64, len(chunk))
			for idx, p := range chunk {
				messageIDs[idx] = p.MessageID
			}

			var sizes map[int64]int64
			if batchResolver, ok := session.(telegramstore.BatchMetadataSession); ok {
				sizes, err = batchResolver.BatchMetadata(ctx, channelID, messageIDs)
			} else {
				sizes = make(map[int64]int64, len(chunk))
				for _, p := range chunk {
					meta, mErr := session.Metadata(ctx, telegramstore.MetadataRequest{
						UserID: p.UserID, ChannelID: p.ChannelID, MessageID: p.MessageID,
					})
					if mErr == nil {
						sizes[p.MessageID] = meta.Size
					}
				}
			}

			if err != nil {
				slog.Warn("backfill.batch_failed", "user_id", userID, "channel_id", channelID, "error", err)
				skippedTotal += int64(len(chunk))
				continue
			}

			fileUpdates := make(map[uuid.UUID]map[int32][2]int64)
			for _, p := range chunk {
				storedSize, ok := sizes[p.MessageID]
				if !ok {
					skippedTotal++
					continue
				}
				plainSize := storedSize
				if p.Encryption {
					decrypted, decErr := contentcrypto.DecryptedSize(storedSize)
					if decErr != nil {
						slog.Warn("backfill.decrypt_size_failed", "file_id", p.FileID, "part_no", p.PartNo, "error", decErr)
						skippedTotal++
						continue
					}
					plainSize = decrypted
				}
				if fileUpdates[p.FileID] == nil {
					fileUpdates[p.FileID] = make(map[int32][2]int64)
				}
				fileUpdates[p.FileID][p.PartNo] = [2]int64{plainSize, storedSize}
			}

			chunkUpdated := int64(0)
			for fID, partSizes := range fileUpdates {
				if updateErr := a.catalog.UpdatePartSizesMany(ctx, fID, partSizes); updateErr != nil {
					slog.Error("backfill.update_db_failed", "file_id", fID, "error", updateErr)
				} else {
					chunkUpdated += int64(len(partSizes))
				}
			}
			updatedTotal += chunkUpdated

			pct := float64(processedTotal) / float64(targetCount) * 100.0
			elapsed := time.Since(startTime)
			eta := time.Duration(0)
			if processedTotal > 0 {
				eta = time.Duration(float64(elapsed) / float64(processedTotal) * float64(targetCount-processedTotal))
			}
			fmt.Printf("[%5.1f%%] Updated: %d | Skipped: %d | Processed: %d/%d | Elapsed: %s | ETA: %s\n",
				pct, updatedTotal, skippedTotal, processedTotal, targetCount,
				elapsed.Round(time.Second), eta.Round(time.Second))

			if delay > 0 {
				select {
				case <-ctx.Done():
				case <-time.After(delay):
				}
			}
		}
	}

	fmt.Printf("\nBackfill finished! Processed: %d, Updated: %d, Skipped/Gone: %d, Total Time: %s\n",
		processedTotal, updatedTotal, skippedTotal, time.Since(startTime).Round(time.Second))
	slog.Info("backfill.finished", "processed", processedTotal, "updated", updatedTotal, "skipped", skippedTotal, "duration", time.Since(startTime))
	return nil
}
