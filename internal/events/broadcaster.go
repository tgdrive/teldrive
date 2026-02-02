package events

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/google/uuid"
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

const (
	// Redis channel name for events
	redisChannel = "teldrive:events"
	// Reconnect delay
	reconnectDelay = 5 * time.Second
	// Default values (used if not configured)
	defaultDBWorkers        = 10
	defaultDBBufferSize     = 1000
	defaultDeduplicationTTL = 30 * time.Minute
)

// EventBroadcaster defines the interface for event broadcasting
type EventBroadcaster interface {
	Subscribe(userID int64) chan models.Event
	Unsubscribe(userID int64, ch chan models.Event)
	Record(eventType EventType, userID int64, source *models.Source)
	Shutdown()
}

// BroadcasterConfig holds configuration for event broadcasting
type BroadcasterConfig struct {
	DBWorkers        int
	DBBufferSize     int
	DeduplicationTTL time.Duration
}

// DefaultBroadcasterConfig returns default configuration
func DefaultBroadcasterConfig() BroadcasterConfig {
	return BroadcasterConfig{
		DBWorkers:        defaultDBWorkers,
		DBBufferSize:     defaultDBBufferSize,
		DeduplicationTTL: defaultDeduplicationTTL,
	}
}

// baseBroadcaster contains shared functionality
type baseBroadcaster struct {
	db           *gorm.DB
	logger       *zap.Logger
	ctx          context.Context
	cancel       context.CancelFunc
	subscribers  map[int64][]chan models.Event
	subMu        sync.RWMutex
	wg           sync.WaitGroup
	dbWorkerCh   chan models.Event
	recentEvents map[string]time.Time // Event ID -> timestamp for deduplication
	eventMu      sync.RWMutex
	config       BroadcasterConfig
}

// newBaseBroadcaster creates a new base broadcaster with DB worker pool
func newBaseBroadcaster(db *gorm.DB, logger *zap.Logger, ctx context.Context, cancel context.CancelFunc, config BroadcasterConfig) *baseBroadcaster {
	// Apply defaults if not set
	if config.DBWorkers <= 0 {
		config.DBWorkers = defaultDBWorkers
	}
	if config.DBBufferSize <= 0 {
		config.DBBufferSize = defaultDBBufferSize
	}
	if config.DeduplicationTTL <= 0 {
		config.DeduplicationTTL = defaultDeduplicationTTL
	}

	b := &baseBroadcaster{
		db:           db,
		logger:       logger,
		ctx:          ctx,
		cancel:       cancel,
		subscribers:  make(map[int64][]chan models.Event),
		dbWorkerCh:   make(chan models.Event, config.DBBufferSize),
		recentEvents: make(map[string]time.Time),
		config:       config,
	}

	// Start DB worker pool
	for i := 0; i < config.DBWorkers; i++ {
		b.wg.Add(1)
		go b.dbWorker()
	}

	return b
}

// dbWorker processes events from the queue and saves to DB
func (b *baseBroadcaster) dbWorker() {
	defer b.wg.Done()
	for {
		select {
		case <-b.ctx.Done():
			return
		case evt := <-b.dbWorkerCh:
			if err := b.db.Create(&evt).Error; err != nil {
				b.logger.Error("events.db_save_failed",
					zap.Error(err),
					zap.String("id", evt.ID),
					zap.String("type", evt.Type),
					zap.Int64("user_id", evt.UserID))
			}
		}
	}
}

// shouldProcess checks if event should be processed (deduplication)
func (b *baseBroadcaster) shouldProcess(eventID string) bool {
	b.eventMu.Lock()
	defer b.eventMu.Unlock()

	// Check if recently processed
	if ts, ok := b.recentEvents[eventID]; ok {
		if time.Since(ts) < b.config.DeduplicationTTL {
			return false // Duplicate
		}
		// Expired, remove old entry
		delete(b.recentEvents, eventID)
	}

	// Mark as processed
	b.recentEvents[eventID] = time.Now()

	// Cleanup old entries periodically (every 100 inserts, but skip if empty)
	if len(b.recentEvents) > 0 && len(b.recentEvents)%100 == 0 {
		b.cleanupOldEvents()
	}

	return true
}

// cleanupOldEvents removes expired entries from deduplication map
func (b *baseBroadcaster) cleanupOldEvents() {
	now := time.Now()
	for id, ts := range b.recentEvents {
		if now.Sub(ts) > b.config.DeduplicationTTL {
			delete(b.recentEvents, id)
		}
	}
}

// broadcast sends event to all local subscribers of a user
func (b *baseBroadcaster) broadcast(evt models.Event) {
	b.subMu.RLock()
	subs, ok := b.subscribers[evt.UserID]
	b.subMu.RUnlock()

	if !ok || len(subs) == 0 {
		return
	}

	sent := 0
	for i, ch := range subs {
		select {
		case ch <- evt:
			sent++
		default:
			b.logger.Debug("events.channel_full",
				zap.String("id", evt.ID),
				zap.Int("subscriber_index", i))
		}
	}
}

// Subscribe creates a new subscription for a user
func (b *baseBroadcaster) Subscribe(userID int64) chan models.Event {
	ch := make(chan models.Event, 100)

	b.subMu.Lock()
	b.subscribers[userID] = append(b.subscribers[userID], ch)
	b.subMu.Unlock()

	b.logger.Debug("events.subscribed",
		zap.Int64("user_id", userID),
		zap.Int("total_subs", len(b.subscribers[userID])))

	return ch
}

// Unsubscribe removes a subscription for a user with graceful drain
func (b *baseBroadcaster) Unsubscribe(userID int64, ch chan models.Event) {
	b.subMu.Lock()

	if subs, ok := b.subscribers[userID]; ok {
		for i, sub := range subs {
			if sub == ch {
				b.subscribers[userID] = append(subs[:i], subs[i+1:]...)
				break
			}
		}
		if len(b.subscribers[userID]) == 0 {
			delete(b.subscribers, userID)
		}
	}

	b.subMu.Unlock()

	// Graceful drain - consume remaining events before closing
	go func() {
		timeout := time.After(100 * time.Millisecond)
		for {
			select {
			case <-ch:
				// Drain
			case <-timeout:
				close(ch)
				return
			}
		}
	}()

	b.logger.Debug("events.unsubscribed",
		zap.Int64("user_id", userID))
}

// queueForDB queues event for DB write (non-blocking)
func (b *baseBroadcaster) queueForDB(evt models.Event) bool {
	select {
	case b.dbWorkerCh <- evt:
		return true
	default:
		b.logger.Warn("events.db_queue_full",
			zap.String("id", evt.ID),
			zap.String("type", evt.Type),
			zap.Int64("user_id", evt.UserID),
			zap.Int("buffer_size", b.config.DBBufferSize))
		return false
	}
}

// createEvent creates a new event from parameters with generated ID
func createEvent(eventType EventType, userID int64, source *models.Source) models.Event {
	return models.Event{
		ID:        uuid.New().String(),
		Type:      string(eventType),
		UserID:    userID,
		Source:    datatypes.NewJSONType(source),
		CreatedAt: time.Now().UTC(),
	}
}

// marshalEvent marshals event to JSON
func marshalEvent(evt models.Event) ([]byte, error) {
	return json.Marshal(evt)
}
