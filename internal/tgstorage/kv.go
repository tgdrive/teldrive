package tgstorage

import (
	"context"

	"github.com/go-faster/errors"
	"gorm.io/gorm"

	"github.com/gotd/contrib/auth/kv"
)

type KeyValue struct {
	Key   string `gorm:"primaryKey;column:key"`
	Value []byte `gorm:"not null;column:value;type:blob"`
}

func (KeyValue) TableName() string {
	return "kv"
}

type kvStorage struct {
	db *gorm.DB
}

func (s kvStorage) Set(ctx context.Context, k, v string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&KeyValue{
			Key:   k,
			Value: []byte(v),
		}).Error; err != nil {
			return errors.Wrap(err, "save value")
		}
		return nil
	})
}

func (s kvStorage) Get(ctx context.Context, k string) (string, error) {
	var entry KeyValue
	if err := s.db.First(&entry, "key = ?", k).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", kv.ErrKeyNotFound
		}
		return "", errors.Wrap(err, "query")
	}
	return string(entry.Value), nil
}
