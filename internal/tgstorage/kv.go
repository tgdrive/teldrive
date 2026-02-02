package tgstorage

import (
	"context"
	"time"

	"github.com/go-faster/errors"
	"github.com/tgdrive/teldrive/internal/cache"
	"gorm.io/gorm"

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
	db    *gorm.DB
	cache cache.Cacher
}

func (s kvStorage) Set(ctx context.Context, k, v string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`
			INSERT INTO teldrive.kv (key, value, created_at)
			VALUES (?, ?, ?)
			ON CONFLICT (key) DO UPDATE SET
				value = EXCLUDED.value,
				created_at = EXCLUDED.created_at
		`, k, []byte(v), time.Now().UTC()).Error; err != nil {
			return errors.Wrap(err, "upsert value")
		}
		return nil
	})
}

func (s kvStorage) Get(ctx context.Context, key string) (string, error) {
	return cache.Fetch(ctx, s.cache, cache.Key(key), 30*time.Minute, func() (string, error) {
		var entry KeyValue
		if err := s.db.First(&entry, "key = ?", key).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return "", kv.ErrKeyNotFound
			}
			return "", errors.Wrap(err, "query")
		}
		return string(entry.Value), nil
	})

}

func (s kvStorage) Delete(ctx context.Context, k string) error {
	if err := s.db.Where("key = ?", k).Delete(&KeyValue{}).Error; err != nil {
		return errors.Wrap(err, "delete key")
	}
	s.cache.Delete(ctx, k)
	return nil
}
