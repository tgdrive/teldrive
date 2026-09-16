package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/tgdrive/teldrive/v2/internal/db/sqlcgen"
	"github.com/tgdrive/teldrive/v2/internal/dbtypes"
	"github.com/tgdrive/teldrive/v2/internal/telegramstore"
)

const (
	UploadCleanupSweepKind = "teldrive_cleanup_uploads"
	CleanupQueue           = "maintenance"
)

var ErrUploadCleanupNotConfigured = errors.New("upload cleanup worker is not configured")

// Deprecated compatibility names; new code should use the explicit upload-cleanup names.
const CleanupSweepKind = UploadCleanupSweepKind

var ErrCleanupNotConfigured = ErrUploadCleanupNotConfigured

type UploadCleanupSweepArgs struct{}

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

type cleanupChannel struct {
	userID    int64
	channelID int64
}

type cleanupPartRecord struct {
	UploadID  uuid.UUID `json:"upload_id"`
	PartNo    int32     `json:"part_no"`
	MessageID int64     `json:"message_id"`
}

func NewUploadCleanupWorker(pool *pgxpool.Pool, storage telegramstore.Storage) *UploadCleanupWorker {
	return &UploadCleanupWorker{pool: pool, queries: sqlcgen.New(pool), storage: storage}
}

func (w *UploadCleanupWorker) Timeout(*river.Job[UploadCleanupSweepArgs]) time.Duration {
	return 2 * time.Hour
}

func (w *UploadCleanupWorker) Work(ctx context.Context, job *river.Job[UploadCleanupSweepArgs]) error {
	if w.pool == nil || w.storage == nil {
		return ErrUploadCleanupNotConfigured
	}
	for {
		expired, err := w.queries.ExpireUploadSessions(ctx)
		if err != nil {
			return fmt.Errorf("expire upload sessions: %w", err)
		}
		sessions, err := w.queries.ListUploadSessionsPendingCleanup(ctx)
		if err != nil {
			return fmt.Errorf("list upload cleanup sessions: %w", err)
		}
		if err := w.cleanupUploads(ctx, sessions); err != nil {
			return err
		}
		if len(expired) == 0 && len(sessions) == 0 {
			return nil
		}
	}
}

func (w *UploadCleanupWorker) cleanupUploads(ctx context.Context, sessions []*sqlcgen.UploadSession) error {
	if len(sessions) == 0 {
		return nil
	}
	userByUpload := make(map[uuid.UUID]int64, len(sessions))
	uploadIDs := make([]pgtype.UUID, 0, len(sessions))
	for _, session := range sessions {
		uploadID, ok := dbtypes.GoogleUUID(session.ID)
		if !ok {
			return errors.New("cleanup session has invalid upload id")
		}
		userByUpload[uploadID] = session.UserID
		uploadIDs = append(uploadIDs, session.ID)
	}
	parts, err := w.queries.ListUploadPartsForCleanupMany(ctx, uploadIDs)
	if err != nil {
		return fmt.Errorf("list upload cleanup parts: %w", err)
	}
	byChannel := make(map[cleanupChannel][]*sqlcgen.UploadPart)
	for _, part := range parts {
		if !part.MessageID.Valid || part.MessageID.Int64 <= 0 {
			continue
		}
		uploadID, ok := dbtypes.GoogleUUID(part.UploadID)
		if !ok {
			return errors.New("cleanup part has invalid upload id")
		}
		userID, ok := userByUpload[uploadID]
		if !ok {
			return errors.New("cleanup part has no upload session")
		}
		key := cleanupChannel{userID: userID, channelID: part.ChannelID}
		byChannel[key] = append(byChannel[key], part)
	}
	channels := make([]cleanupChannel, 0, len(byChannel))
	for channel := range byChannel {
		channels = append(channels, channel)
	}
	sort.Slice(channels, func(i, j int) bool {
		if channels[i].userID == channels[j].userID {
			return channels[i].channelID < channels[j].channelID
		}
		return channels[i].userID < channels[j].userID
	})
	for _, channel := range channels {
		channelParts := byChannel[channel]
		messageIDs := make([]int64, 0, len(channelParts))
		records := make([]cleanupPartRecord, 0, len(channelParts))
		for _, part := range channelParts {
			messageIDs = append(messageIDs, part.MessageID.Int64)
			uploadID, _ := dbtypes.GoogleUUID(part.UploadID)
			records = append(records, cleanupPartRecord{UploadID: uploadID, PartNo: part.PartNo, MessageID: part.MessageID.Int64})
		}
		if err := w.storage.DeleteMessages(ctx, channel.userID, channel.channelID, messageIDs); err != nil {
			return fmt.Errorf("delete Telegram upload messages for user %d channel %d: %w", channel.userID, channel.channelID, err)
		}
		encoded, err := json.Marshal(records)
		if err != nil {
			return fmt.Errorf("encode cleaned upload parts: %w", err)
		}
		deleted, err := w.queries.DeleteUploadPartsForCleanup(ctx, encoded)
		if err != nil {
			return fmt.Errorf("delete upload parts after Telegram cleanup: %w", err)
		}
		if deleted != int64(len(records)) {
			return fmt.Errorf("%d upload parts changed during cleanup", int64(len(records))-deleted)
		}
	}
	return nil
}
