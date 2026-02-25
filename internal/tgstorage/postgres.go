package tgstorage

import (
	"context"
	"time"

	"github.com/go-faster/errors"
	"github.com/gotd/contrib/auth/kv"
	"github.com/gotd/td/session"
	"github.com/tgdrive/teldrive/internal/cache"
	jetmodel "github.com/tgdrive/teldrive/internal/database/jetgen/teldrive_jet/teldrive/model"
	"github.com/tgdrive/teldrive/pkg/repositories"
)

var _ Storage = (*PostgresStorage)(nil)

// PostgresStorage implements session storage using repository KV storage
type PostgresStorage struct {
	kv    repositories.KVRepository
	cache cache.Cacher
	key   string
}

// NewPostgresStorage creates a new PostgreSQL-backed session storage with caching
func NewPostgresStorage(kvRepo repositories.KVRepository, cache cache.Cacher, key string) *PostgresStorage {
	return &PostgresStorage{
		kv:    kvRepo,
		cache: cache,
		key:   key,
	}
}

// LoadSession retrieves session data from PostgreSQL with caching
func (s *PostgresStorage) LoadSession(ctx context.Context) ([]byte, error) {
	// Use cache if available
	if s.cache != nil {
		return cache.Fetch(ctx, s.cache, cache.Key("session", s.key), 30*time.Minute, func() ([]byte, error) {
			entry, err := s.kv.Get(ctx, s.key)
			if err != nil {
				if errors.Is(err, repositories.ErrNotFound) {
					return nil, session.ErrNotFound
				}
				return nil, errors.Wrap(err, "get session")
			}
			return entry.Value, nil
		})
	}

	entry, err := s.kv.Get(ctx, s.key)
	if err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			return nil, session.ErrNotFound
		}
		return nil, errors.Wrap(err, "get session")
	}

	return entry.Value, nil
}

// StoreSession saves session data to PostgreSQL
func (s *PostgresStorage) StoreSession(ctx context.Context, data []byte) error {
	item := &jetmodel.Kv{Key: s.key, Value: data, CreatedAt: time.Now().UTC()}
	if err := s.kv.Set(ctx, item); err != nil {
		return errors.Wrap(err, "upsert session")
	}

	if s.cache != nil {
		s.cache.Delete(ctx, cache.Key("session", s.key))
	}

	return nil
}

// Type returns the storage type
func (s *PostgresStorage) Type() string {
	return "postgres"
}

// Close is a no-op for PostgreSQL storage (connection managed elsewhere)
func (s *PostgresStorage) Close() error {
	return nil
}

// PostgresSession wraps kv.Session for PostgreSQL backend with cache
type PostgresSession struct {
	kv.Session
}

// NewPostgresSession creates a new PostgreSQL-backed session with optional caching
func NewPostgresSession(kvRepo repositories.KVRepository, cache cache.Cacher, key string) *PostgresSession {
	storage := &kvStorage{
		kv:    kvRepo,
		cache: cache,
	}
	return &PostgresSession{
		Session: kv.NewSession(storage, key),
	}
}
