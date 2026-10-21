package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tgdrive/teldrive/v2/internal/config"
)

func TestNewDownloadCacheDisabled(t *testing.T) {
	for _, directory := range []string{"", "  "} {
		cfg := config.Default().Cache.Disk
		cfg.Directory = directory
		cache, err := newDownloadCache(cfg)
		if err != nil || cache != nil {
			t.Fatalf("disabled cache for directory %q = %v, %v", directory, cache, err)
		}
	}
}

func TestNewDownloadCacheLifecycle(t *testing.T) {
	cfg := config.Default().Cache.Disk
	cfg.Directory = t.TempDir()
	cache, err := newDownloadCache(cfg)
	if err != nil {
		t.Fatal(err)
	}
	application := &App{downloadCache: cache}
	if err := application.Close(); err != nil {
		t.Fatal(err)
	}
	if err := application.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(cfg.Directory, "data")); err != nil {
		t.Fatal(err)
	}
}

func TestNewDownloadCacheReportsInitializationErrors(t *testing.T) {
	cfg := config.Default().Cache.Disk
	cfg.Directory = filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(cfg.Directory, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if cache, err := newDownloadCache(cfg); err == nil || cache != nil {
		t.Fatalf("invalid cache directory = %v, %v", cache, err)
	}
}
