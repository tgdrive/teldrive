package cache

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
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

func NewCache(ctx context.Context, conf *config.CacheConfig) Cacher {
	var cacher Cacher
	if conf.RedisAddr == "" {
		cacher = NewMemoryCache(conf.MaxSize)
	} else {
		cacher = NewRedisCache(ctx, redis.NewClient(&redis.Options{
			Addr:            conf.RedisAddr,
			Password:        conf.RedisPass,
			DialTimeout:     5 * time.Second,
			ReadTimeout:     3 * time.Second,
			WriteTimeout:    3 * time.Second,
			PoolSize:        10,
			MinIdleConns:    5,
			MaxIdleConns:    10,
			ConnMaxIdleTime: 5 * time.Minute,
			ConnMaxLifetime: 1 * time.Hour,
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

func Fetch[T any](cache Cacher, key string, expiration time.Duration, fn func() (T, error)) (T, error) {
	var zero, value T
	err := cache.Get(key, &value)
	if err != nil {
		if errors.Is(err, freecache.ErrNotFound) || errors.Is(err, redis.Nil) {
			value, err = fn()
			if err != nil {
				return zero, err
			}
			cache.Set(key, &value, expiration)
			return value, nil
		}
		return zero, err
	}
	return value, nil
}

func Key(args ...interface{}) string {
	parts := make([]string, len(args))
	for i, arg := range args {
		parts[i] = formatValue(arg)
	}
	return strings.Join(parts, ":")
}

func formatValue(v interface{}) string {
	if v == nil {
		return "nil"
	}

	val := reflect.ValueOf(v)
	switch val.Kind() {
	case reflect.Ptr:
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
		return fmt.Sprintf("{%s}", strings.Join(parts, ","))
	case reflect.Struct:
		return fmt.Sprintf("%+v", v)
	default:
		return fmt.Sprintf("%v", v)
	}
}
