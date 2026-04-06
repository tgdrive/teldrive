package events

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/tgdrive/teldrive/pkg/repositories"
	"go.uber.org/zap"
)

func NewBroadcaster(ctx context.Context, eventsRepo repositories.EventRepository, redisClient *redis.Client, pollInterval time.Duration, config BroadcasterConfig, logger *zap.Logger) EventBroadcaster {
	if redisClient != nil {
		logger.Debug("events.using_redis_broadcaster")
		return NewRedisBroadcaster(ctx, eventsRepo, redisClient, config, logger)
	}

	logger.Debug("events.using_polling_broadcaster", zap.Duration("poll_interval", pollInterval))
	return NewPollingBroadcaster(ctx, eventsRepo, pollInterval, config, logger)
}
