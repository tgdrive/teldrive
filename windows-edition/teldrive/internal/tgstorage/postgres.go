package tgstorage

import (
	"context"
	"time"

	"github.com/go-faster/errors"
	"github.com/gotd/contrib/auth/kv"
	"github.com/gotd/td/session"
	"github.com/tgdrive/teldrive/internal/cache"
	"gorm.io/gorm"
)

var _ Storage = (*PostgresStorage)(nil)

// PostgresStorage implements session storage using PostgreSQL via GORM
type PostgresStorage struct {
	db    *gorm.DB
	cache cache.Cacher
	key   string
}

// NewPostgresStorage creates a new PostgreSQL-backed session storage with caching
func NewPostgresStorage(db *gorm.DB, cache cache.Cacher, key string) *PostgresStorage {
	return &PostgresStorage{
		db:    db,
		cache: cache,
		key:   key,
	}
}

// LoadSession retrieves session data from PostgreSQL with caching
func (s *PostgresStorage) LoadSession(ctx context.Context) ([]byte, error) {
	// Use cache if available
	if s.cache != nil {
		return cache.Fetch(ctx, s.cache, cache.Key("session", s.key), 30*time.Minute, func() ([]byte, error) {
			var entry KeyValue
			if err := s.db.WithContext(ctx).First(&entry, "key = ?", s.key).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return nil, session.ErrNotFound
				}
				return nil, errors.Wrap(err, "query session")
			}
			return entry.Value, nil
		})
	}

	// Fallback to direct DB query
	var entry KeyValue
	if err := s.db.WithContext(ctx).First(&entry, "key = ?", s.key).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, session.ErrNotFound
		}
		return nil, errors.Wrap(err, "query session")
	}
	return entry.Value, nil
}

// StoreSession saves session data to PostgreSQL
func (s *PostgresStorage) StoreSession(ctx context.Context, data []byte) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`
			INSERT INTO teldrive.kv (key, value, created_at)
			VALUES (?, ?, ?)
			ON CONFLICT (key) DO UPDATE SET
				value = EXCLUDED.value,
				created_at = EXCLUDED.created_at
		`, s.key, data, time.Now().UTC()).Error; err != nil {
			return errors.Wrap(err, "upsert session")
		}
		return nil
	})
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
func NewPostgresSession(db *gorm.DB, cache cache.Cacher, key string) *PostgresSession {
	storage := &kvStorage{
		db:    db,
		cache: cache,
	}
	return &PostgresSession{
		Session: kv.NewSession(storage, key),
	}
}
