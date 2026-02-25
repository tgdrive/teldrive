package events

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/tgdrive/teldrive/internal/api"
	"github.com/tgdrive/teldrive/pkg/dto"
	"github.com/tgdrive/teldrive/pkg/repositories"
	"go.uber.org/zap"
)

type EventType = api.EventType

const (
	OpCreate         EventType = api.EventTypeFilesCreated
	OpUpdate         EventType = api.EventTypeFilesUpdated
	OpDelete         EventType = api.EventTypeFilesDeleted
	OpMove           EventType = api.EventTypeFilesMoved
	OpCopy           EventType = api.EventTypeFilesCopied
	OpUploadProgress EventType = api.EventTypeUploadsProgress
	OpJobProgress    EventType = api.EventTypeJobsProgress
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
	Subscribe(userID int64, eventTypes []EventType) chan dto.Event
	Unsubscribe(userID int64, ch chan dto.Event)
	Record(eventType EventType, userID int64, source *dto.Source)
	Shutdown()
}

type eventSubscriber struct {
	ch      chan dto.Event
	filters map[EventType]struct{}
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
	eventsRepo   repositories.EventRepository
	logger       *zap.Logger
	ctx          context.Context
	cancel       context.CancelFunc
	subscribers  map[int64][]eventSubscriber
	subMu        sync.RWMutex
	wg           sync.WaitGroup
	dbWorkerCh   chan dto.Event
	recentEvents map[string]time.Time // Event ID -> timestamp for deduplication
	eventMu      sync.RWMutex
	config       BroadcasterConfig
}

// newBaseBroadcaster creates a new base broadcaster with DB worker pool
func newBaseBroadcaster(eventsRepo repositories.EventRepository, logger *zap.Logger, ctx context.Context, cancel context.CancelFunc, config BroadcasterConfig) *baseBroadcaster {
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
		eventsRepo:   eventsRepo,
		logger:       logger,
		ctx:          ctx,
		cancel:       cancel,
		subscribers:  make(map[int64][]eventSubscriber),
		dbWorkerCh:   make(chan dto.Event, config.DBBufferSize),
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
			eventModel, err := eventToModel(evt)
			if err != nil {
				b.logger.Error("events.model_mapping_failed", zap.Error(err))
				continue
			}

			if err := b.eventsRepo.Create(b.ctx, eventModel); err != nil {
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
func (b *baseBroadcaster) broadcast(evt dto.Event) {
	b.subMu.RLock()
	subs, ok := b.subscribers[evt.UserID]
	b.subMu.RUnlock()

	if !ok || len(subs) == 0 {
		return
	}

	sent := 0
	eventType := EventType(evt.Type)
	for i, sub := range subs {
		if len(sub.filters) > 0 {
			if _, ok := sub.filters[eventType]; !ok {
				continue
			}
		}

		select {
		case sub.ch <- evt:
			sent++
		default:
			b.logger.Debug("events.channel_full",
				zap.String("id", evt.ID),
				zap.Int("subscriber_index", i))
		}
	}
}

// Subscribe creates a new subscription for a user
func (b *baseBroadcaster) Subscribe(userID int64, eventTypes []EventType) chan dto.Event {
	ch := make(chan dto.Event, 100)
	filters := make(map[EventType]struct{}, len(eventTypes))
	for _, eventType := range eventTypes {
		filters[eventType] = struct{}{}
	}

	b.subMu.Lock()
	b.subscribers[userID] = append(b.subscribers[userID], eventSubscriber{ch: ch, filters: filters})
	b.subMu.Unlock()

	b.logger.Debug("events.subscribed",
		zap.Int64("user_id", userID),
		zap.Int("total_subs", len(b.subscribers[userID])))

	return ch
}

// Unsubscribe removes a subscription for a user with graceful drain
func (b *baseBroadcaster) Unsubscribe(userID int64, ch chan dto.Event) {
	b.subMu.Lock()

	if subs, ok := b.subscribers[userID]; ok {
		for i, sub := range subs {
			if sub.ch == ch {
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
func (b *baseBroadcaster) queueForDB(evt dto.Event) bool {
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
func createEvent(eventType EventType, userID int64, source *dto.Source) dto.Event {
	return dto.Event{
		ID:        uuid.New().String(),
		Type:      string(eventType),
		UserID:    userID,
		Source:    source,
		CreatedAt: time.Now().UTC(),
	}
}

// marshalEvent marshals event to JSON
func marshalEvent(evt dto.Event) ([]byte, error) {
	return json.Marshal(evt)
}
