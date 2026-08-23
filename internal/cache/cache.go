package cache

import (
	"context"
	"fmt"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/coocood/freecache"
	"github.com/vmihailenco/msgpack/v5"
)

// Cacher is the global in-memory cache contract restored from v1.8.3.
// Redis variant intentionally removed — production uses only freecache memory.
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

// NewMemoryCache creates a freecache-backed memory cache.
// size is bytes, e.g., 2*1024*1024 for 2MB. Must be >= 512KB; always >0 via config default.
func NewMemoryCache(size int) *MemoryCache {
	return &MemoryCache{
		cache:  freecache.NewCache(size),
		prefix: "teldrive:",
	}
}

// NewCache is compatibility helper from v1.8.3. Now only memory is supported.
func NewCache(_ context.Context, maxSize int) Cacher {
	return NewMemoryCache(maxSize)
}

func (m *MemoryCache) Get(_ context.Context, key string, value any) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	key = m.prefix + key
	data, err := m.cache.Get([]byte(key))
	if err != nil {
		return err
	}
	return msgpack.Unmarshal(data, value)
}

func (m *MemoryCache) Set(_ context.Context, key string, value any, expiration time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	key = m.prefix + key
	data, err := msgpack.Marshal(value)
	if err != nil {
		return err
	}
	return m.cache.Set([]byte(key), data, int(expiration.Seconds()))
}

func (m *MemoryCache) Delete(_ context.Context, keys ...string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, key := range keys {
		m.cache.Del([]byte(m.prefix + key))
	}
	return nil
}

func (m *MemoryCache) DeletePattern(_ context.Context, pattern string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	pattern = m.prefix + pattern
	iter := m.cache.NewIterator()
	for {
		entry := iter.Next()
		if entry == nil {
			break
		}
		k := string(entry.Key)
		if matched, _ := filepath.Match(pattern, k); matched {
			m.cache.Del(entry.Key)
		}
	}
	return nil
}

// Fetch is generic read-through helper: try cache, on miss call fn and cache result.
func Fetch[T any](ctx context.Context, c Cacher, key string, expiration time.Duration, fn func() (T, error)) (T, error) {
	var zero, value T
	err := c.Get(ctx, key, &value)
	if err == nil {
		return value, nil
	}
	if err != freecache.ErrNotFound {
		// treat any error as miss if it's ErrNotFound, otherwise return
		// freecache only returns ErrNotFound or nil; keep strict for future Cacher impls
		var isNotFound bool
		if err == freecache.ErrNotFound {
			isNotFound = true
		}
		if !isNotFound {
			// check wrapped not-found via string match for compatibility
			if strings.Contains(err.Error(), "not found") {
				isNotFound = true
			}
		}
		if !isNotFound {
			return zero, err
		}
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
