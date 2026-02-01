package events

import (
	"context"

	"go.uber.org/zap"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/tgdrive/teldrive/pkg/models"
)

type EventType string

const (
	OpCreate EventType = "file_create"
	OpUpdate EventType = "file_update"
	OpDelete EventType = "file_delete"
	OpMove   EventType = "file_move"
	OpCopy   EventType = "file_copy"
)

type Recorder struct {
	db     *gorm.DB
	events chan models.Event
	logger *zap.Logger
	ctx    context.Context
	done   chan struct{}
}

func NewRecorder(ctx context.Context, db *gorm.DB, logger *zap.Logger) *Recorder {
	r := &Recorder{
		db:     db,
		events: make(chan models.Event, 1000),
		logger: logger,
		ctx:    ctx,
		done:   make(chan struct{}),
	}

	go r.processEvents()
	return r
}

func (r *Recorder) Record(eventType EventType, userID int64, source *models.Source) {
	evt := models.Event{
		Type:   string(eventType),
		UserID: userID,
		Source: datatypes.NewJSONType(source),
	}

	// Try to send to channel (non-blocking)
	select {
	case r.events <- evt:
		// Event queued successfully
	default:
		// Channel full, log warning but still save synchronously to not lose events
		r.logger.Warn("events.queue_full_saving_sync",
			zap.String("type", string(eventType)),
			zap.Int64("user_id", userID))
		// Save synchronously to avoid losing events
		if err := r.db.Create(&evt).Error; err != nil {
			r.logger.Error("events.sync_save_failed",
				zap.Error(err),
				zap.String("type", string(evt.Type)),
				zap.Int64("user_id", evt.UserID))
		}
	}
}

func (r *Recorder) processEvents() {
	defer close(r.done)
	for {
		select {
		case <-r.ctx.Done():
			r.drainEvents()
			return
		case evt, ok := <-r.events:
			if !ok {
				return
			}
			if err := r.db.Create(&evt).Error; err != nil {
				r.logger.Error("events.save_failed",
					zap.Error(err),
					zap.String("type", string(evt.Type)), //nolint:unconvert
					zap.Int64("user_id", evt.UserID))
			}
		}
	}
}

func (r *Recorder) drainEvents() {
	for {
		select {
		case evt, ok := <-r.events:
			if !ok {
				return
			}
			if err := r.db.Create(&evt).Error; err != nil {
				r.logger.Error("events.save_failed_shutdown",
					zap.Error(err),
					zap.String("type", string(evt.Type)), //nolint:unconvert
					zap.Int64("user_id", evt.UserID))
			}
		default:
			return
		}
	}
}

func (r *Recorder) Shutdown() {
	close(r.events)
	<-r.done
}
