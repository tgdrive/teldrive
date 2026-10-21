package config

import (
	"testing"
	"time"
)

func TestDiskCacheDefaults(t *testing.T) {
	cfg := Default().Cache.Disk
	if cfg.Directory != "" || cfg.MaxSize != 10<<30 || cfg.MinFreeSpace != 1<<30 {
		t.Fatalf("disk cache defaults = %#v", cfg)
	}
	if cfg.ChunkSize != 64<<20 || cfg.ChunkSizeLimit != 1<<30 || cfg.ChunkStreams != 0 || cfg.PollInterval != time.Minute {
		t.Fatalf("disk cache tuning defaults = %#v", cfg)
	}
}

func TestLoadDiskCacheEnvironment(t *testing.T) {
	values := map[string]string{
		"TELDRIVE_DATABASE_URL":             "postgres://example/teldrive",
		"TELDRIVE_SECURITY_SIGNING_KEY":     "0123456789abcdef0123456789abcdef",
		"TELDRIVE_SECURITY_DATA_KEY":        "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		"TELDRIVE_CACHE_DISK_DIRECTORY":     "/var/cache/teldrive",
		"TELDRIVE_CACHE_DISK_MAX_SIZE":      "20GB",
		"TELDRIVE_CACHE_DISK_CHUNK_STREAMS": "3",
		"TELDRIVE_CACHE_DISK_READ_AHEAD":    "8MB",
		"TELDRIVE_CACHE_DISK_MAX_AGE":       "12h",
	}
	cfg, err := LoadFrom(func(key string) (string, bool) { value, ok := values[key]; return value, ok })
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Cache.Disk.Directory != "/var/cache/teldrive" || cfg.Cache.Disk.MaxSize != 20<<30 || cfg.Cache.Disk.ChunkStreams != 3 || cfg.Cache.Disk.ReadAhead != 8<<20 || cfg.Cache.Disk.MaxAge != 12*time.Hour {
		t.Fatalf("loaded disk cache = %#v", cfg.Cache.Disk)
	}
}

func TestValidateDiskCache(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*DiskCache)
	}{
		{"negative size", func(c *DiskCache) { c.MaxSize = -1 }},
		{"zero poll interval", func(c *DiskCache) { c.PollInterval = 0 }},
		{"negative age", func(c *DiskCache) { c.MaxAge = -time.Second }},
		{"zero chunk", func(c *DiskCache) { c.ChunkSize = 0 }},
		{"inverted chunk sizes", func(c *DiskCache) { c.ChunkSizeLimit = 1 }},
		{"too many streams", func(c *DiskCache) { c.ChunkStreams = 33 }},
		{"invalid sharding", func(c *DiskCache) { c.ShardDepth = 17 }},
		{"zero retries", func(c *DiskCache) { c.Retries = 0 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := validTestConfig()
			cfg.Cache.Disk.Directory = "/var/cache/teldrive"
			tc.mutate(&cfg.Cache.Disk)
			if err := cfg.Validate(); err == nil {
				t.Fatal("invalid disk cache configuration accepted")
			}
		})
	}
}
