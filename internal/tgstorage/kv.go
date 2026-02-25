package tgstorage

import (
	"context"
	"time"

	"github.com/go-faster/errors"
	"github.com/tgdrive/teldrive/internal/cache"
	jetmodel "github.com/tgdrive/teldrive/internal/database/jetgen/teldrive_jet/teldrive/model"
	"github.com/tgdrive/teldrive/pkg/repositories"

	"github.com/gotd/contrib/auth/kv"
)

type KeyValue struct {
	Key       string    `gorm:"type:text;primaryKey"`
	Value     []byte    `gorm:"type:bytea;not null"`
	CreatedAt time.Time `gorm:"default:timezone('utc'::text, now())"`
}

func (KeyValue) TableName() string {
	return "teldrive.kv"
}

type kvStorage struct {
	kv    repositories.KVRepository
	cache cache.Cacher
}

func (s kvStorage) Set(ctx context.Context, k, v string) error {
	if err := s.kv.Set(ctx, &jetmodel.Kv{Key: k, Value: []byte(v), CreatedAt: time.Now().UTC()}); err != nil {
		return errors.Wrap(err, "upsert value")
	}
	if s.cache != nil {
		s.cache.Delete(ctx, cache.Key(k))
	}
	return nil
}

func (s kvStorage) Get(ctx context.Context, key string) (string, error) {
	// Skip cache if not configured
	if s.cache == nil {
		entry, err := s.kv.Get(ctx, key)
		if err != nil {
			if errors.Is(err, repositories.ErrNotFound) {
				return "", kv.ErrKeyNotFound
			}
			return "", errors.Wrap(err, "query")
		}
		return string(entry.Value), nil
	}

	return cache.Fetch(ctx, s.cache, cache.Key(key), 30*time.Minute, func() (string, error) {
		entry, err := s.kv.Get(ctx, key)
		if err != nil {
			if errors.Is(err, repositories.ErrNotFound) {
				return "", kv.ErrKeyNotFound
			}
			return "", errors.Wrap(err, "query")
		}
		return string(entry.Value), nil
	})
}

func (s kvStorage) Delete(ctx context.Context, k string) error {
	if err := s.kv.Delete(ctx, k); err != nil {
		return errors.Wrap(err, "delete key")
	}
	if s.cache != nil {
		s.cache.Delete(ctx, k)
	}
	return nil
}
