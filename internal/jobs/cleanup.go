package jobs

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/tgdrive/teldrive/v2/internal/db/sqlcgen"
	"github.com/tgdrive/teldrive/v2/internal/dbtypes"
	"github.com/tgdrive/teldrive/v2/internal/telegramstore"
)

const (
	UploadCleanupSweepKind = "teldrive_cleanup_uploads"
	CleanupQueue           = "maintenance"
	defaultBatchSize       = 100
)

var ErrUploadCleanupNotConfigured = errors.New("upload cleanup worker is not configured")

// Deprecated compatibility names; new code should use the explicit upload-cleanup names.
const CleanupSweepKind = UploadCleanupSweepKind

var ErrCleanupNotConfigured = ErrUploadCleanupNotConfigured

type UploadCleanupSweepArgs struct {
	BatchSize int32 `json:"batch_size,omitempty"`
}

type CleanupSweepArgs = UploadCleanupSweepArgs

func (UploadCleanupSweepArgs) Kind() string { return UploadCleanupSweepKind }

func (UploadCleanupSweepArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue:       CleanupQueue,
		MaxAttempts: 3,
		Priority:    2,
	}
}

type UploadCleanupWorker struct {
	river.WorkerDefaults[UploadCleanupSweepArgs]
	pool    *pgxpool.Pool
	queries *sqlcgen.Queries
	storage telegramstore.Storage
}

func NewUploadCleanupWorker(pool *pgxpool.Pool, storage telegramstore.Storage) *UploadCleanupWorker {
	return &UploadCleanupWorker{pool: pool, queries: sqlcgen.New(pool), storage: storage}
}

func (w *UploadCleanupWorker) Timeout(*river.Job[UploadCleanupSweepArgs]) time.Duration {
	return 10 * time.Minute
}

func (w *UploadCleanupWorker) Work(ctx context.Context, job *river.Job[UploadCleanupSweepArgs]) error {
	if w.pool == nil || w.storage == nil {
		return ErrUploadCleanupNotConfigured
	}
	batchSize := job.Args.BatchSize
	if batchSize <= 0 {
		batchSize = defaultBatchSize
	}
	if _, err := w.queries.ExpireUploadSessions(ctx, batchSize); err != nil {
		return fmt.Errorf("expire upload sessions: %w", err)
	}
	sessions, err := w.queries.ListUploadSessionsPendingCleanup(ctx, batchSize)
	if err != nil {
		return fmt.Errorf("list upload cleanup sessions: %w", err)
	}
	for _, session := range sessions {
		uploadID, ok := dbtypes.GoogleUUID(session.ID)
		if !ok {
			return errors.New("cleanup session has invalid upload id")
		}
		if err := w.cleanupUpload(ctx, session.UserID, uploadID); err != nil {
			return err
		}
	}
	return nil
}

func (w *UploadCleanupWorker) cleanupUpload(ctx context.Context, userID int64, uploadID uuid.UUID) error {
	parts, err := w.queries.ListUploadPartsForCleanup(ctx, dbtypes.UUID(uploadID))
	if err != nil {
		return fmt.Errorf("list cleanup parts for upload %s: %w", uploadID, err)
	}
	byChannel := make(map[int64][]*sqlcgen.UploadPart)
	for _, part := range parts {
		if !part.MessageID.Valid || part.MessageID.Int64 <= 0 {
			continue
		}
		byChannel[part.ChannelID] = append(byChannel[part.ChannelID], part)
	}
	channelIDs := make([]int64, 0, len(byChannel))
	for channelID := range byChannel {
		channelIDs = append(channelIDs, channelID)
	}
	sort.Slice(channelIDs, func(i, j int) bool { return channelIDs[i] < channelIDs[j] })
	for _, channelID := range channelIDs {
		channelParts := byChannel[channelID]
		messageIDs := make([]int64, 0, len(channelParts))
		for _, part := range channelParts {
			messageIDs = append(messageIDs, part.MessageID.Int64)
		}
		if err := w.storage.DeleteMessages(ctx, userID, channelID, messageIDs); err != nil {
			return fmt.Errorf("delete Telegram upload messages for upload %s channel %d: %w", uploadID, channelID, err)
		}
		for _, part := range channelParts {
			deleted, err := w.queries.DeleteUploadPartForCleanup(ctx, sqlcgen.DeleteUploadPartForCleanupParams{
				UploadID:  dbtypes.UUID(uploadID),
				PartNo:    part.PartNo,
				MessageID: part.MessageID,
			})
			if err != nil {
				return fmt.Errorf("delete upload part %d after Telegram cleanup: %w", part.PartNo, err)
			}
			if deleted == 0 {
				return fmt.Errorf("upload part %d changed during cleanup", part.PartNo)
			}
		}
	}
	return nil
}
