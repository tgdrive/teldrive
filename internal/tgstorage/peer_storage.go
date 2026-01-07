package tgstorage

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/go-faster/errors"
	"github.com/gotd/contrib/storage"
	"github.com/tgdrive/teldrive/internal/cache"
	"gorm.io/gorm"
)

var _ storage.PeerStorage = PeerStorage{}

type PeerStorage struct {
	db     *gorm.DB
	prefix string
}

func NewPeerStorage(db *gorm.DB, prefix string) *PeerStorage {
	return &PeerStorage{
		db:     db,
		prefix: prefix,
	}
}

type postgresIterator struct {
	rows  *sql.Rows
	value storage.Peer
}

func (p *postgresIterator) Close() error {
	return p.rows.Close()
}

func (p *postgresIterator) Next(ctx context.Context) bool {
	if !p.rows.Next() {
		return false
	}

	var val []byte
	if err := p.rows.Scan(&val); err != nil {
		return false
	}

	if err := json.Unmarshal(val, &p.value); err != nil {
		if errors.Is(err, storage.ErrPeerUnmarshalMustInvalidate) {
			return p.Next(ctx)
		}
		return false
	}

	return true
}

func (p *postgresIterator) Err() error {
	return p.rows.Err()
}

func (p *postgresIterator) Value() storage.Peer {
	return p.value
}

func (s PeerStorage) Iterate(ctx context.Context) (storage.PeerIterator, error) {
	rows, err := s.db.Model(&KeyValue{}).
		Select("value").
		Where("key LIKE ?", s.prefix+"%").
		Rows()
	if err != nil {
		return nil, errors.Wrap(err, "query")
	}

	return &postgresIterator{rows: rows}, nil
}

func (s PeerStorage) Purge(ctx context.Context) error {
	err := s.db.Where("key LIKE ?", s.prefix+"%").Delete(&KeyValue{})
	if err != nil {
		return err.Error
	}
	return nil
}

func (s PeerStorage) add(associated []string, value storage.Peer) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		data, err := json.Marshal(value)
		if err != nil {
			return errors.Wrap(err, "marshal")
		}

		if err := tx.Save(&KeyValue{
			Key:   cache.Key(s.prefix, storage.KeyFromPeer(value).String()),
			Value: data,
		}).Error; err != nil {
			return errors.Wrap(err, "save peer")
		}

		for _, key := range associated {
			if err := tx.Save(&KeyValue{
				Key:       cache.Key(s.prefix, key),
				Value:     data,
				CreatedAt: time.Now().UTC(),
			}).Error; err != nil {
				return errors.Wrap(err, "save associated key")
			}
		}

		return nil
	})
}

func (s PeerStorage) Add(ctx context.Context, value storage.Peer) error {
	return s.add(value.Keys(), value)
}

func (s PeerStorage) Find(ctx context.Context, key storage.PeerKey) (storage.Peer, error) {
	var entry KeyValue
	if err := s.db.First(&entry, "key = ?", cache.Key(s.prefix, key.String())).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return storage.Peer{}, storage.ErrPeerNotFound
		}
		return storage.Peer{}, errors.Wrap(err, "query")
	}

	var p storage.Peer
	if err := json.Unmarshal(entry.Value, &p); err != nil {
		if errors.Is(err, storage.ErrPeerUnmarshalMustInvalidate) {
			return storage.Peer{}, storage.ErrPeerNotFound
		}
		return storage.Peer{}, errors.Wrap(err, "unmarshal")
	}

	return p, nil
}

func (s PeerStorage) Delete(ctx context.Context, key storage.PeerKey) error {
	if err := s.db.Where("key = ?", cache.Key(s.prefix, key.String())).Delete(&KeyValue{}).Error; err != nil {
		return errors.Wrap(err, "query")
	}
	return nil
}

func (s PeerStorage) Assign(ctx context.Context, key string, value storage.Peer) error {
	return s.add(append(value.Keys(), key), value)
}

func (s PeerStorage) Resolve(ctx context.Context, key string) (storage.Peer, error) {
	var entry KeyValue
	if err := s.db.First(&entry, "key = ?", cache.Key(s.prefix, key)).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return storage.Peer{}, storage.ErrPeerNotFound
		}
		return storage.Peer{}, errors.Wrap(err, "query")
	}

	var p storage.Peer
	if err := json.Unmarshal(entry.Value, &p); err != nil {
		if errors.Is(err, storage.ErrPeerUnmarshalMustInvalidate) {
			return storage.Peer{}, storage.ErrPeerNotFound
		}
		return storage.Peer{}, errors.Wrap(err, "unmarshal")
	}

	return p, nil
}
