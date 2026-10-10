package events

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

func NewBroadcaster(ctx context.Context, db *gorm.DB, redisClient *redis.Client, pollInterval time.Duration, config BroadcasterConfig, logger *zap.Logger) EventBroadcaster {
	if redisClient != nil {
		logger.Debug("events.using_redis_broadcaster")
		return NewRedisBroadcaster(ctx, db, redisClient, config, logger)
	}

	logger.Debug("events.using_polling_broadcaster", zap.Duration("poll_interval", pollInterval))
	return NewPollingBroadcaster(ctx, db, pollInterval, config, logger)
}
