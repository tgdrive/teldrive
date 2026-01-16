package cache

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/tgdrive/teldrive/internal/config"
)

// NewRedisClient creates a Redis client from config.
// Returns nil if Redis is not configured (Addr is empty).
func NewRedisClient(ctx context.Context, conf *config.RedisConfig) (*redis.Client, error) {
	if conf.Addr == "" {
		return nil, nil
	}
	client := redis.NewClient(&redis.Options{
		Addr:            conf.Addr,
		Password:        conf.Password,
		DialTimeout:     5 * time.Second,
		ReadTimeout:     3 * time.Second,
		WriteTimeout:    3 * time.Second,
		PoolSize:        conf.PoolSize,
		MinIdleConns:    conf.MinIdleConns,
		MaxIdleConns:    conf.MaxIdleConns,
		ConnMaxIdleTime: conf.ConnMaxIdleTime,
		ConnMaxLifetime: conf.ConnMaxLifetime,
	})

	if err := client.Ping(ctx).Err(); err != nil {
		client.Close()
		return nil, err
	}

	return client, nil
}
