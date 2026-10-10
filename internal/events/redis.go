package events

import (
	"context"
	"encoding/json"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/tgdrive/teldrive/pkg/models"
)

// RedisBroadcaster implements EventBroadcaster using Redis Pub/Sub for distributed setups
type RedisBroadcaster struct {
	*baseBroadcaster
	redisClient *redis.Client
}

// NewRedisBroadcaster creates a new Redis-based event broadcaster
func NewRedisBroadcaster(ctx context.Context, db *gorm.DB, redisClient *redis.Client, config BroadcasterConfig, logger *zap.Logger) *RedisBroadcaster {
	ctx, cancel := context.WithCancel(ctx)
	b := &RedisBroadcaster{
		baseBroadcaster: newBaseBroadcaster(db, logger, ctx, cancel, config),
		redisClient:     redisClient,
	}

	b.wg.Add(1)
	go b.subscribe()

	logger.Info("events.redis_broadcaster_created")
	return b
}

// subscribe connects to Redis and listens for events from all server instances
func (b *RedisBroadcaster) subscribe() {
	defer b.wg.Done()

	for {
		select {
		case <-b.ctx.Done():
			return
		default:
		}

		b.logger.Info("events.subscribing", zap.String("channel", redisChannel))

		pubsub := b.redisClient.Subscribe(b.ctx, redisChannel)
		_, err := pubsub.Receive(b.ctx)
		if err != nil {
			b.logger.Error("events.subscribe_failed", zap.Error(err))
			pubsub.Close()
			select {
			case <-b.ctx.Done():
				return
			case <-time.After(reconnectDelay):
				continue
			}
		}

		b.logger.Info("events.subscribed", zap.String("channel", redisChannel))

		ch := pubsub.Channel()
	innerLoop:
		for {
			select {
			case <-b.ctx.Done():
				pubsub.Close()
				// Drain any pending messages before returning
				for len(ch) > 0 {
					<-ch
				}
				return
			case msg, ok := <-ch:
				if !ok {
					b.logger.Warn("events.channel_closed")
					pubsub.Close()
					// Check context before reconnecting
					select {
					case <-b.ctx.Done():
						return
					case <-time.After(reconnectDelay):
						break innerLoop // Break to outer loop for reconnect
					}
				}

				var evt models.Event
				if err := json.Unmarshal([]byte(msg.Payload), &evt); err != nil {
					b.logger.Error("events.failed_to_unmarshal",
						zap.Error(err),
						zap.String("payload", msg.Payload))
					continue
				}

				// Deduplication check
				if !b.shouldProcess(evt.ID) {
					b.logger.Debug("events.duplicate_skipped",
						zap.String("id", evt.ID))
					continue
				}

				b.logger.Debug("events.received",
					zap.String("id", evt.ID),
					zap.Int64("user_id", evt.UserID),
					zap.String("type", evt.Type))

				b.broadcast(evt)
			}
		}
	}
}

// Record saves an event to the database and publishes it to Redis
// Does NOT broadcast locally - the subscribe() loop will handle broadcasting
// when the message comes back from Redis (ensuring single broadcast)
func (b *RedisBroadcaster) Record(eventType EventType, userID int64, source *models.Source) {
	evt := createEvent(eventType, userID, source)

	// Queue for DB write (non-blocking)
	if !b.queueForDB(evt) {
		// Queue full, log and continue - Redis publish will still happen
		b.logger.Warn("events.db_queue_full_on_record",
			zap.String("id", evt.ID),
			zap.Int64("user_id", userID))
	}

	// Publish to Redis asynchronously
	go func() {
		payload, err := marshalEvent(evt)
		if err != nil {
			b.logger.Error("events.failed_to_marshal", zap.Error(err))
			return
		}

		if err := b.redisClient.Publish(b.ctx, redisChannel, payload).Err(); err != nil {
			b.logger.Error("events.redis_publish_failed", zap.Error(err))
			return
		}

		b.logger.Debug("events.published",
			zap.String("id", evt.ID),
			zap.Int64("user_id", userID),
			zap.String("type", string(eventType)))
	}()
}

// Shutdown gracefully stops the broadcaster
func (b *RedisBroadcaster) Shutdown() {
	b.logger.Info("events.redis_broadcaster_shutting_down")
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

	b.logger.Info("events.redis_broadcaster_shutdown_complete")
}
