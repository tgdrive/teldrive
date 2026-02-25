package tgstorage

import (
	"context"
	"encoding/json"

	"github.com/go-faster/errors"
	"github.com/gotd/contrib/storage"
	"github.com/tgdrive/teldrive/internal/cache"
	"github.com/tgdrive/teldrive/internal/database/jetgen/teldrive_jet/teldrive/model"
	"github.com/tgdrive/teldrive/pkg/repositories"
)

var _ storage.PeerStorage = PeerStorage{}

type PeerStorage struct {
	kv     repositories.KVRepository
	prefix string
}

func NewPeerStorage(kvRepo repositories.KVRepository, prefix string) *PeerStorage {
	return &PeerStorage{
		kv:     kvRepo,
		prefix: prefix,
	}
}

type postgresIterator struct {
	values []storage.Peer
	index  int
}

func (p *postgresIterator) Close() error {
	return nil
}

func (p *postgresIterator) Next(ctx context.Context) bool {
	if p.index >= len(p.values) {
		return false
	}
	p.index++
	return true
}

func (p *postgresIterator) Err() error {
	return nil
}

func (p *postgresIterator) Value() storage.Peer {
	if p.index == 0 || p.index > len(p.values) {
		return storage.Peer{}
	}
	return p.values[p.index-1]
}

func (s PeerStorage) Iterate(ctx context.Context) (storage.PeerIterator, error) {
	values := make([]storage.Peer, 0)
	err := s.kv.Iterate(ctx, s.prefix, func(key string, value []byte) error {
		var p storage.Peer
		if err := json.Unmarshal(value, &p); err != nil {
			if errors.Is(err, storage.ErrPeerUnmarshalMustInvalidate) {
				return nil
			}
			return err
		}
		values = append(values, p)
		return nil
	})
	if err != nil {
		return nil, errors.Wrap(err, "query")
	}

	return &postgresIterator{values: values}, nil
}

func (s PeerStorage) Purge(ctx context.Context) error {
	return s.kv.DeletePrefix(ctx, s.prefix)
}

func (s PeerStorage) add(associated []string, value storage.Peer) error {
	data, err := json.Marshal(value)
	if err != nil {
		return errors.Wrap(err, "marshal")
	}

	if err := s.kv.Set(context.Background(), &model.Kv{Key: cache.Key(s.prefix, storage.KeyFromPeer(value).String()), Value: data}); err != nil {
		return errors.Wrap(err, "save peer")
	}

	for _, key := range associated {
		if err := s.kv.Set(context.Background(), &model.Kv{Key: cache.Key(s.prefix, key), Value: data}); err != nil {
			return errors.Wrap(err, "save associated key")
		}
	}

	return nil
}

func (s PeerStorage) Add(ctx context.Context, value storage.Peer) error {
	return s.add(value.Keys(), value)
}

func (s PeerStorage) Find(ctx context.Context, key storage.PeerKey) (storage.Peer, error) {
	entry, err := s.kv.Get(ctx, cache.Key(s.prefix, key.String()))
	if err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
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
	if err := s.kv.Delete(ctx, cache.Key(s.prefix, key.String())); err != nil {
		return errors.Wrap(err, "query")
	}
	return nil
}

func (s PeerStorage) Assign(ctx context.Context, key string, value storage.Peer) error {
	return s.add(append(value.Keys(), key), value)
}

func (s PeerStorage) Resolve(ctx context.Context, key string) (storage.Peer, error) {
	entry, err := s.kv.Get(ctx, cache.Key(s.prefix, key))
	if err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
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
