package tgstorage

import (
	"fmt"

	"github.com/gotd/td/session"
	"github.com/tgdrive/teldrive/internal/cache"
	"github.com/tgdrive/teldrive/internal/config"
	"github.com/tgdrive/teldrive/pkg/repositories"
)

// Storage defines the interface for all session storage backends
// It wraps gotd/td/session.Storage with additional lifecycle methods
type Storage interface {
	session.Storage // LoadSession, StoreSession

	// Type returns the storage backend type
	Type() string

	// Close closes the storage backend
	Close() error
}

// NewSessionStorage creates a session storage based on configuration
// key identifies the specific session (e.g., userID, bot token)
// cache is used for PostgreSQL storage to reduce DB reads
func NewSessionStorage(cfg config.SessionStorageConfig, kvRepo repositories.KVRepository, cache cache.Cacher, key string) (Storage, error) {
	switch cfg.Type {
	case "bolt":
		return NewBoltStorage(cfg.Bolt, key)
	case "postgres", "postgresql", "pg":
		return NewPostgresStorage(kvRepo, cache, key), nil
	case "memory", "":
		return NewMemoryStorage(), nil
	default:
		return nil, fmt.Errorf("unknown session storage type: %s", cfg.Type)
	}
}
