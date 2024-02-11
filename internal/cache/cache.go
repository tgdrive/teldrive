package cache

import (
	"context"
	"sync"
	"time"

	"github.com/coocood/freecache"
	"github.com/gin-gonic/gin"
	"github.com/vmihailenco/msgpack"
)

type Cache struct {
	cache *freecache.Cache
	mu    sync.RWMutex
}

func (c *Cache) Get(key string, value interface{}) error {
	c.mu.RLock()
	defer c.mu.RUnlock()
	result, err := c.cache.Get([]byte(key))
	if err != nil {
		return err
	}

	err = msgpack.Unmarshal(result, value)

	if err != nil {
		return err
	}
	return nil
}

func (c *Cache) Set(key string, value interface{}, expires time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	bytes, err := msgpack.Marshal(value)
	if err != nil {
		return err
	}
	return c.cache.Set([]byte(key), bytes, int(expires.Seconds()))
}

func (c *Cache) Delete(key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.cache.Del([]byte(key))
	return nil
}

var (
	defaultCache     *Cache
	defaultCacheOnce sync.Once
)

type Config struct {
	Size int
}

var conf = &Config{
	Size: 5 * 1024 * 1024,
}

func SetConfig(c *Config) {
	conf = &Config{
		Size: c.Size,
	}
}

func DefaultCache() *Cache {
	defaultCacheOnce.Do(func() {
		defaultCache = &Cache{cache: freecache.NewCache(conf.Size)}
	})
	return defaultCache
}

type cacheKeyType string

var contextKey = cacheKeyType("cache")

func WithCache(ctx context.Context, cache *Cache) context.Context {
	if gCtx, ok := ctx.(*gin.Context); ok {
		ctx = gCtx.Request.Context()
	}
	return context.WithValue(ctx, contextKey, cache)
}

func FromContext(ctx context.Context) *Cache {
	if ctx == nil {
		return DefaultCache()
	}
	if gCtx, ok := ctx.(*gin.Context); ok && gCtx != nil {
		ctx = gCtx.Request.Context()
	}
	if cache, ok := ctx.Value(contextKey).(*Cache); ok {
		return cache
	}
	return DefaultCache()
}
