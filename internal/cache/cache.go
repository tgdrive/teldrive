package cache

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/coocood/freecache"
	"github.com/redis/go-redis/v9"
	"github.com/vmihailenco/msgpack/v5"
)

type Cacher interface {
	Get(ctx context.Context, key string, value any) error
	Set(ctx context.Context, key string, value any, expiration time.Duration) error
	Delete(ctx context.Context, keys ...string) error
	DeletePattern(ctx context.Context, pattern string) error
}

type MemoryCache struct {
	cache  *freecache.Cache
	prefix string
	mu     sync.RWMutex
}

// NewCache creates a new cache instance.
// If redisClient is provided, uses Redis; otherwise falls back to in-memory cache.
func NewCache(ctx context.Context, maxSize int, redisClient *redis.Client) Cacher {
	if redisClient != nil {
		return NewRedisCache(redisClient)
	}
	return NewMemoryCache(maxSize)
}

func NewMemoryCache(size int) *MemoryCache {
	return &MemoryCache{
		cache:  freecache.NewCache(size),
		prefix: "teldrive:",
	}
}

func (m *MemoryCache) Get(ctx context.Context, key string, value any) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	key = m.prefix + key
	data, err := m.cache.Get([]byte(key))
	if err != nil {
		return err
	}
	return msgpack.Unmarshal(data, value)
}

func (m *MemoryCache) Set(ctx context.Context, key string, value any, expiration time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	key = m.prefix + key
	data, err := msgpack.Marshal(value)
	if err != nil {
		return err
	}
	return m.cache.Set([]byte(key), data, int(expiration.Seconds()))
}

func (m *MemoryCache) Delete(ctx context.Context, keys ...string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, key := range keys {
		m.cache.Del([]byte(m.prefix + key))
	}
	return nil
}

func (m *MemoryCache) DeletePattern(ctx context.Context, pattern string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	pattern = m.prefix + pattern
	iter := m.cache.NewIterator()
	for {
		entry := iter.Next()
		if entry == nil {
			break
		}
		key := string(entry.Key)
		if matched, _ := filepath.Match(pattern, key); matched {
			m.cache.Del(entry.Key)
		}
	}
	return nil
}

type RedisCache struct {
	client *redis.Client
	prefix string
}

func NewRedisCache(client *redis.Client) *RedisCache {
	return &RedisCache{
		client: client,
		prefix: "teldrive:",
	}
}

func (r *RedisCache) Get(ctx context.Context, key string, value any) error {
	key = r.prefix + key
	data, err := r.client.Get(ctx, key).Bytes()
	if err != nil {
		return err
	}
	return msgpack.Unmarshal(data, value)
}

func (r *RedisCache) Set(ctx context.Context, key string, value any, expiration time.Duration) error {
	key = r.prefix + key
	data, err := msgpack.Marshal(value)
	if err != nil {
		return err
	}
	return r.client.Set(ctx, key, data, expiration).Err()
}

func (r *RedisCache) Delete(ctx context.Context, keys ...string) error {
	for i := range keys {
		keys[i] = r.prefix + keys[i]
	}
	return r.client.Del(ctx, keys...).Err()
}

func (r *RedisCache) DeletePattern(ctx context.Context, pattern string) error {
	pattern = r.prefix + pattern
	iter := r.client.Scan(ctx, 0, pattern, 0).Iterator()
	var errs []error
	for iter.Next(ctx) {
		if err := r.client.Del(ctx, iter.Val()).Err(); err != nil {
			errs = append(errs, err)
		}
	}
	if err := iter.Err(); err != nil {
		errs = append(errs, err)
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

func Fetch[T any](ctx context.Context, cache Cacher, key string, expiration time.Duration, fn func() (T, error)) (T, error) {
	var zero, value T
	err := cache.Get(ctx, key, &value)
	if err != nil {
		if errors.Is(err, freecache.ErrNotFound) || errors.Is(err, redis.Nil) {
			value, err = fn()
			if err != nil {
				return zero, err
			}
			cache.Set(ctx, key, &value, expiration)
			return value, nil
		}
		return zero, err
	}
	return value, nil
}

func FetchArg[T any, A any](
	ctx context.Context,
	cache Cacher,
	key string,
	expiration time.Duration,
	fn func(a A) (T, error), a A) (T, error) {
	return Fetch(ctx, cache, key, expiration, func() (T, error) {
		return fn(a)
	})
}

func Key(args ...any) string {
	parts := make([]string, len(args))
	for i, arg := range args {
		parts[i] = formatValue(arg)
	}
	return strings.Join(parts, ":")
}

func formatValue(v any) string {
	if v == nil {
		return "nil"
	}

	val := reflect.ValueOf(v)
	switch val.Kind() {
	case reflect.Pointer:
		if val.IsNil() {
			return "nil"
		}
		return formatValue(val.Elem().Interface())
	case reflect.Array, reflect.Slice:
		parts := make([]string, val.Len())
		for i := 0; i < val.Len(); i++ {
			parts[i] = formatValue(val.Index(i).Interface())
		}
		return fmt.Sprintf("[%s]", strings.Join(parts, ","))
	case reflect.Map:
		parts := make([]string, 0, val.Len())
		for _, key := range val.MapKeys() {
			k := formatValue(key.Interface())
			v := formatValue(val.MapIndex(key).Interface())
			parts = append(parts, fmt.Sprintf("%s=%s", k, v))
		}
		sort.Strings(parts)
		return fmt.Sprintf("{%s}", strings.Join(parts, ","))
	case reflect.Struct:
		return fmt.Sprintf("%+v", v)
	default:
		return fmt.Sprintf("%v", v)
	}
}
