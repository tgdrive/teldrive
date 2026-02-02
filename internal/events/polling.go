package events

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/tgdrive/teldrive/pkg/models"
)

// PollingBroadcaster implements EventBroadcaster using polling for single-instance setups
type PollingBroadcaster struct {
	*baseBroadcaster
	pollInterval time.Duration
	lastPollTime time.Time
	polling      bool
	pollMu       sync.Mutex
}

// NewPollingBroadcaster creates a new polling-based event broadcaster for single-instance setups
// Uses lazy polling - only starts polling when there are subscribers
func NewPollingBroadcaster(ctx context.Context, db *gorm.DB, pollInterval time.Duration, config BroadcasterConfig, logger *zap.Logger) *PollingBroadcaster {
	if pollInterval <= 0 {
		pollInterval = 10 * time.Second // Default: 10 seconds
	}

	ctx, cancel := context.WithCancel(ctx)
	b := &PollingBroadcaster{
		baseBroadcaster: newBaseBroadcaster(db, logger, ctx, cancel, config),
		pollInterval:    pollInterval,
		lastPollTime:    time.Now(), // Start from current time (don't query old events)
		polling:         false,      // Lazy polling - don't start until there are subscribers
	}

	logger.Debug("events.polling_broadcaster_created",
		zap.Duration("poll_interval", pollInterval),
		zap.Bool("lazy_polling", true))
	return b
}

// startPolling starts the polling goroutine if not already running
func (b *PollingBroadcaster) startPolling() {
	b.pollMu.Lock()
	defer b.pollMu.Unlock()

	if b.polling {
		return // Already polling
	}

	b.polling = true
	b.lastPollTime = time.Now() // Reset to current time when starting
	b.wg.Add(1)
	go b.poll()

	b.logger.Debug("events.polling_started",
		zap.Int("total_subscribers", b.getTotalSubscribers()))
}

// stopPolling stops the polling goroutine
func (b *PollingBroadcaster) stopPolling() {
	b.pollMu.Lock()
	defer b.pollMu.Unlock()

	if !b.polling {
		return // Not polling
	}

	b.polling = false
	// Note: We don't cancel the context here, just stop the ticker loop
	// The context is managed by baseBroadcaster

	b.logger.Debug("events.polling_stopped",
		zap.Int("total_subscribers", b.getTotalSubscribers()))
}

// getTotalSubscribers returns the total number of subscribers across all users
func (b *PollingBroadcaster) getTotalSubscribers() int {
	b.subMu.RLock()
	defer b.subMu.RUnlock()

	total := 0
	for _, subs := range b.subscribers {
		total += len(subs)
	}
	return total
}

// poll periodically checks the database for new events
func (b *PollingBroadcaster) poll() {
	defer b.wg.Done()

	ticker := time.NewTicker(b.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-b.ctx.Done():
			return
		case <-ticker.C:
			b.pollMu.Lock()
			isPolling := b.polling
			b.pollMu.Unlock()

			if !isPolling {
				return // Stop polling
			}
			b.checkForNewEvents()
		}
	}
}

// checkForNewEvents queries the database for events since last poll
func (b *PollingBroadcaster) checkForNewEvents() {
	now := time.Now()

	// Query events from all users (limit to prevent memory issues if server was down)
	var events []models.Event
	if err := b.db.Where("created_at > ?", b.lastPollTime).Order("created_at ASC").Limit(1000).Find(&events).Error; err != nil {
		b.logger.Error("events.poll_query_failed", zap.Error(err))
		return
	}

	b.lastPollTime = now

	if len(events) == 0 {
		return
	}

	// Broadcast to subscribers with deduplication
	for _, evt := range events {
		// Skip if already processed (deduplication)
		if !b.shouldProcess(evt.ID) {
			continue
		}
		b.broadcast(evt)
	}
}

// Subscribe creates a new subscription for a user and starts polling if needed
func (b *PollingBroadcaster) Subscribe(userID int64) chan models.Event {
	ch := make(chan models.Event, 100)

	b.subMu.Lock()
	b.subscribers[userID] = append(b.subscribers[userID], ch)
	totalSubs := 0
	for _, subs := range b.subscribers {
		totalSubs += len(subs)
	}
	b.subMu.Unlock()

	b.logger.Debug("events.subscribed",
		zap.Int64("user_id", userID),
		zap.Int("user_subs", len(b.subscribers[userID])),
		zap.Int("total_subs", totalSubs))

	// Start polling on first subscriber
	if totalSubs == 1 {
		b.startPolling()
	}

	return ch
}

// Unsubscribe removes a subscription for a user and stops polling if no subscribers left
func (b *PollingBroadcaster) Unsubscribe(userID int64, ch chan models.Event) {
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

	totalSubs := 0
	for _, subs := range b.subscribers {
		totalSubs += len(subs)
	}
	b.subMu.Unlock()

	b.logger.Debug("events.unsubscribed",
		zap.Int64("user_id", userID),
		zap.Int("total_subs", totalSubs))

	// Stop polling when no subscribers left
	if totalSubs == 0 {
		b.stopPolling()
	}

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
}

// Record saves an event to the database (no immediate broadcast - let poll discover it)
func (b *PollingBroadcaster) Record(eventType EventType, userID int64, source *models.Source) {
	evt := createEvent(eventType, userID, source)
	// ID is already generated by createEvent()

	b.broadcast(evt)
	// Only save to DB - poll() will discover and broadcast it
	// This prevents duplicate broadcasts
	select {
	case b.dbWorkerCh <- evt:
		// Queued for DB write
	default:
		b.logger.Warn("events.db_queue_full",
			zap.Int64("user_id", userID),
			zap.String("type", string(eventType)))
	}
}

// Shutdown gracefully stops the broadcaster
func (b *PollingBroadcaster) Shutdown() {

	// Stop polling first
	b.stopPolling()

	// Then cancel context and wait
	b.cancel()

	// Wait for workers with timeout to prevent hanging
	done := make(chan struct{})
	go func() {
		b.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Normal shutdown
	case <-time.After(5 * time.Second):
		b.logger.Warn("events.shutdown_timeout")
	}

}
