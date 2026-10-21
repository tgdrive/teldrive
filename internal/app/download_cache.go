package app

import (
	"context"
	"fmt"
	"strings"

	varccache "github.com/tgdrive/varc/cache"

	"github.com/tgdrive/teldrive/v2/internal/config"
)

func newDownloadCache(cfg config.DiskCache) (*varccache.Cache, error) {
	if strings.TrimSpace(cfg.Directory) == "" {
		return nil, nil
	}
	directory, err := expandHomePath(cfg.Directory)
	if err != nil {
		return nil, fmt.Errorf("resolve disk cache directory: %w", err)
	}
	cache, err := varccache.New(context.Background(), directory, varccache.Options{
		CachePollInterval: cfg.PollInterval, CacheMaxAge: cfg.MaxAge,
		CacheMaxSize: int64(cfg.MaxSize), CacheMinFreeSpace: int64(cfg.MinFreeSpace), CacheShardDepth: cfg.ShardDepth,
		ChunkSize: int64(cfg.ChunkSize), ChunkSizeLimit: int64(cfg.ChunkSizeLimit), ChunkStreams: cfg.ChunkStreams,
		ReadAhead: int64(cfg.ReadAhead), BufferSize: int64(cfg.BufferSize),
		HandleCaching: cfg.HandleCaching, LowLevelRetries: cfg.Retries,
	})
	if err != nil {
		return nil, fmt.Errorf("create disk download cache: %w", err)
	}
	return cache, nil
}
