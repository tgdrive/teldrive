package cache

import (
	"time"
)

type cacheConfig struct {
	size      int // Size in MB
	ttl       time.Duration
	cleanFreq time.Duration
}

// Interface to wrap any caching implementation
type Cache interface {
	Set(key, value interface{}) error // Only exported fields in struct will be stored
	Get(key interface{}) (interface{}, error)
}

// New builds a new default cache. You may pass options to modify the default values
func New(opts ...Option) (Cache, error) {
	cacheConfig := &cacheConfig{
		size:      1,
		ttl:       60 * time.Second,
		cleanFreq: 30 * time.Second,
	}

	for _, opt := range opts {
		opt.apply(cacheConfig)
	}

	cache, err := newBigCache(cacheConfig)
	if err != nil {
		return nil, err
	}
	return cache, nil
}

type Option interface {
	apply(cacheConfig *cacheConfig)
}

type optionFunc func(*cacheConfig)

func (opt optionFunc) apply(cacheConfig *cacheConfig) {
	opt(cacheConfig)
}

// WithSizeInMB sets the size of the cache in MBs
// The minimum size of the cache is 1 MB
// If a size of 0 or less is passed the cache will have unlimited size
func WithSizeInMB(size int) Option {
	return optionFunc(func(cacheConfig *cacheConfig) {
		cacheConfig.size = size
	})
}

// WithTTL will cause the cache to expire any item that lives longer
// than the given ttl
func WithTTL(ttl time.Duration) Option {
	return optionFunc(func(cacheConfig *cacheConfig) {
		cacheConfig.ttl = ttl
	})
}

// WithCleanFrequency sets how often the cache will clean out expired items
// The lowest the frequency may be is 1 second
// If the time is 0 then no cleaning will happen and items will never be removed
func WithCleanFrequency(cleanFreq time.Duration) Option {
	return optionFunc(func(cacheConfig *cacheConfig) {
		cacheConfig.cleanFreq = cleanFreq
	})
}
