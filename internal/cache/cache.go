package cache

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/dgraph-io/ristretto/v2"
	"github.com/vmihailenco/msgpack/v5"
)

var ErrNotFound = errors.New("cache: not found")

// Cacher is the global in-memory cache contract restored from v1.8.3.
type Cacher interface {
	Get(ctx context.Context, key string, value any) error
	Set(ctx context.Context, key string, value any, expiration time.Duration) error
	Delete(ctx context.Context, keys ...string) error
}

type MemoryCache struct {
	cache  *ristretto.Cache[string, []byte]
	prefix string
}

// NewMemoryCache creates a Ristretto-backed memory cache.
// size is the maximum cache cost in bytes, e.g. 5*1024*1024 for 5MB.
func NewMemoryCache(size int) *MemoryCache {
	numCounters := int64(size/1024) * 10
	if numCounters < 10_000 {
		numCounters = 10_000
	}
	c, err := ristretto.NewCache(&ristretto.Config[string, []byte]{
		NumCounters: numCounters,
		MaxCost:     int64(size),
		BufferItems: 64,
	})
	if err != nil {
		panic(fmt.Sprintf("create memory cache: %v", err))
	}
	return &MemoryCache{cache: c, prefix: "teldrive:"}
}

// NewCache is compatibility helper from v1.8.3. Now only memory is supported.
func NewCache(_ context.Context, maxSize int) Cacher {
	return NewMemoryCache(maxSize)
}

func (m *MemoryCache) Get(_ context.Context, key string, value any) error {
	key = m.prefix + key
	data, ok := m.cache.Get(key)
	if !ok {
		return ErrNotFound
	}
	return msgpack.Unmarshal(data, value)
}

func (m *MemoryCache) Set(_ context.Context, key string, value any, expiration time.Duration) error {
	key = m.prefix + key
	data, err := msgpack.Marshal(value)
	if err != nil {
		return err
	}
	cost := int64(len(key) + len(data))
	if m.cache.SetWithTTL(key, data, cost, expiration) {
		m.cache.Wait()
	}
	return nil
}

func (m *MemoryCache) Delete(_ context.Context, keys ...string) error {
	for _, key := range keys {
		m.cache.Del(m.prefix + key)
	}
	return nil
}

func (m *MemoryCache) Close() {
	if m != nil && m.cache != nil {
		m.cache.Close()
	}
}

// Fetch is generic read-through helper: try cache, on miss call fn and cache result.
func Fetch[T any](ctx context.Context, c Cacher, key string, expiration time.Duration, fn func() (T, error)) (T, error) {
	var zero, value T
	err := c.Get(ctx, key, &value)
	if err == nil {
		return value, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return zero, err
	}
	value, err = fn()
	if err != nil {
		return zero, err
	}
	_ = c.Set(ctx, key, &value, expiration)
	return value, nil
}

func FetchArg[T any, A any](ctx context.Context, c Cacher, key string, expiration time.Duration, fn func(a A) (T, error), a A) (T, error) {
	return Fetch(ctx, c, key, expiration, func() (T, error) {
		return fn(a)
	})
}

// Key builds a colon-joined cache key from arbitrary args, sorted for maps.
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
		for _, k := range val.MapKeys() {
			parts = append(parts, fmt.Sprintf("%s=%s", formatValue(k.Interface()), formatValue(val.MapIndex(k).Interface())))
		}
		sort.Strings(parts)
		return fmt.Sprintf("{%s}", strings.Join(parts, ","))
	case reflect.Struct:
		return fmt.Sprintf("%+v", v)
	default:
		return fmt.Sprintf("%v", v)
	}
}
