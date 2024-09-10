package cache

import (
	"context"
	"sync"
	"time"

	"github.com/coocood/freecache"
	"github.com/redis/go-redis/v9"
	"github.com/tgdrive/teldrive/internal/config"
	"github.com/vmihailenco/msgpack/v5"
)

type Cacher interface {
	Get(key string, value interface{}) error
	Set(key string, value interface{}, expiration time.Duration) error
	Delete(keys ...string) error
}

type MemoryCache struct {
	cache  *freecache.Cache
	prefix string
	mu     sync.RWMutex
}

func NewCache(ctx context.Context, conf *config.Config) Cacher {
	var cacher Cacher
	if conf.Cache.RedisAddr == "" {
		cacher = NewMemoryCache(conf.Cache.MaxSize)
	} else {
		cacher = NewRedisCache(ctx, redis.NewClient(&redis.Options{
			Addr:     conf.Cache.RedisAddr,
			Password: conf.Cache.RedisPass,
		}))
	}
	return cacher
}

func NewMemoryCache(size int) *MemoryCache {
	return &MemoryCache{
		cache:  freecache.NewCache(size),
		prefix: "teldrive:",
	}
}

func (m *MemoryCache) Get(key string, value interface{}) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	key = m.prefix + key
	data, err := m.cache.Get([]byte(key))
	if err != nil {
		return err
	}
	return msgpack.Unmarshal(data, value)
}

func (m *MemoryCache) Set(key string, value interface{}, expiration time.Duration) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	key = m.prefix + key
	data, err := msgpack.Marshal(value)
	if err != nil {
		return err
	}
	return m.cache.Set([]byte(key), data, int(expiration.Seconds()))
}

func (m *MemoryCache) Delete(keys ...string) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, key := range keys {
		m.cache.Del([]byte(m.prefix + key))
	}
	return nil
}

type RedisCache struct {
	client *redis.Client
	ctx    context.Context
	prefix string
	mu     sync.RWMutex
}

func NewRedisCache(ctx context.Context, client *redis.Client) *RedisCache {
	return &RedisCache{
		client: client,
		prefix: "teldrive:",
		ctx:    ctx,
	}
}

func (r *RedisCache) Get(key string, value interface{}) error {
	r.mu.RLock()
	defer r.mu.RUnlock()
	key = r.prefix + key
	data, err := r.client.Get(r.ctx, key).Bytes()
	if err != nil {
		return err
	}
	return msgpack.Unmarshal(data, value)
}

func (r *RedisCache) Set(key string, value interface{}, expiration time.Duration) error {
	r.mu.RLock()
	defer r.mu.RUnlock()
	key = r.prefix + key
	data, err := msgpack.Marshal(value)
	if err != nil {
		return err
	}
	return r.client.Set(r.ctx, key, data, expiration).Err()
}

func (r *RedisCache) Delete(keys ...string) error {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for i := range keys {
		keys[i] = r.prefix + keys[i]
	}
	return r.client.Del(r.ctx, keys...).Err()
}
