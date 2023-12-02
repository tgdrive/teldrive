package cache

import (
	"encoding/json"
	"sync"

	"github.com/coocood/freecache"
)

var cache *Cache

type Cache struct {
	cache *freecache.Cache
	mu    sync.RWMutex
}

func InitCache() {
	cache = &Cache{cache: freecache.NewCache(10 * 1024 * 1024)}
}

func GetCache() *Cache {
	return cache
}

func (c *Cache) Get(key string, value interface{}) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	got, err := cache.cache.Get([]byte(key))

	if err != nil {
		return err
	}

	return json.Unmarshal(got, value)
}

func (c *Cache) Set(key string, value interface{}, expireSeconds int) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	data, err := json.Marshal(value)
	if err != nil {
		return err
	}

	cache.cache.Set([]byte(key), data, expireSeconds)
	return nil
}

func (c *Cache) Delete(key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	cache.cache.Del([]byte(key))
	return nil
}
