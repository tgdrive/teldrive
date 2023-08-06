package cache

import (
	"bytes"
	"context"
	"encoding/gob"
	"errors"

	"github.com/allegro/bigcache/v3"
)

type bigCache struct {
	cache *bigcache.BigCache
}

func newBigCache(cacheConfig *cacheConfig) (*bigCache, error) {
	cache, err := bigcache.New(context.Background(), bigcache.Config{
		Shards:             16,
		LifeWindow:         cacheConfig.ttl,
		CleanWindow:        cacheConfig.cleanFreq,
		MaxEntriesInWindow: 1000 * 10 * 60,
		MaxEntrySize:       500,
		Verbose:            false,
		HardMaxCacheSize:   cacheConfig.size,
		StatsEnabled:       true,
	})
	if err != nil {
		return nil, err
	}
	return &bigCache{
		cache: cache,
	}, nil
}

// Set inserts the key/value pair into the cache.
// Only the exported fields of the given struct will be
// serialized and stored
func (c *bigCache) Set(key, value interface{}) error {
	keyString, ok := key.(string)
	if !ok {
		return errors.New("a cache key must be a string")
	}

	valueBytes, err := serializeGOB(value)
	if err != nil {
		return err
	}

	return c.cache.Set(keyString, valueBytes)
}

// Get returns the value correlating to the key in the cache
func (c *bigCache) Get(key interface{}) (interface{}, error) {
	// Assert the key is of string type
	keyString, ok := key.(string)
	if !ok {
		return nil, errors.New("a cache key must be a string")
	}

	// Get the value in the byte format it is stored in
	valueBytes, err := c.cache.Get(keyString)
	if err != nil {
		return nil, err
	}

	// Deserialize the bytes of the value
	value, err := deserializeGOB(valueBytes)
	if err != nil {
		return nil, err
	}

	return value, nil
}

func serializeGOB(value interface{}) ([]byte, error) {
	buf := bytes.Buffer{}
	enc := gob.NewEncoder(&buf)
	gob.Register(value)

	err := enc.Encode(&value)
	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func deserializeGOB(valueBytes []byte) (interface{}, error) {
	var value interface{}
	buf := bytes.NewBuffer(valueBytes)
	dec := gob.NewDecoder(buf)

	err := dec.Decode(&value)
	if err != nil {
		return nil, err
	}

	return value, nil
}
