package tgstorage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gotd/contrib/auth/kv"
	"github.com/gotd/td/session"
	"github.com/tgdrive/teldrive/internal/config"
	"github.com/tgdrive/teldrive/internal/utils"
	"go.etcd.io/bbolt"
)

var _ Storage = (*BoltStorage)(nil)

// BoltStorage implements session storage using BoltDB
// Uses a single file with separate buckets for different sessions
type BoltStorage struct {
	db  *bbolt.DB
	key string
}

// bucketName is the BoltDB bucket for all sessions
var sessionBucket = []byte("sessions")

// NewBoltStorage creates a new BoltDB session storage
func NewBoltStorage(cfg config.BoltSessionConfig, key string) (*BoltStorage, error) {
	path := cfg.Path
	if path == "" {
		// Auto-detect path
		dir, err := os.UserHomeDir()
		if err != nil {
			dir = utils.ExecutableDir()
		} else {
			dir = filepath.Join(dir, ".teldrive")
			if err := os.MkdirAll(dir, 0755); err != nil {
				dir = utils.ExecutableDir()
			}
		}
		path = filepath.Join(dir, "session.db")
	}

	opts := &bbolt.Options{
		Timeout:    cfg.Timeout,
		NoGrowSync: cfg.NoGrowSync,
	}
	if opts.Timeout == 0 {
		opts.Timeout = 1000 // 1 second default
	}

	db, err := bbolt.Open(path, 0600, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to open bolt db: %w", err)
	}

	// Create bucket if not exists
	err = db.Update(func(tx *bbolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(sessionBucket)
		return err
	})
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create session bucket: %w", err)
	}

	return &BoltStorage{db: db, key: key}, nil
}

// LoadSession retrieves session data from BoltDB
func (s *BoltStorage) LoadSession(ctx context.Context) ([]byte, error) {
	var data []byte
	err := s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(sessionBucket)
		if b == nil {
			return session.ErrNotFound
		}

		val := b.Get([]byte(s.key))
		if val == nil {
			return session.ErrNotFound
		}

		// Copy data (BoltDB reuses memory)
		data = append([]byte{}, val...)
		return nil
	})

	if err != nil {
		return nil, err
	}

	return data, nil
}

// StoreSession saves session data to BoltDB
func (s *BoltStorage) StoreSession(ctx context.Context, data []byte) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(sessionBucket)
		if b == nil {
			return fmt.Errorf("session bucket not found")
		}
		return b.Put([]byte(s.key), data)
	})
}

// Type returns the storage type
func (s *BoltStorage) Type() string {
	return "bolt"
}

// Close closes the BoltDB database
func (s *BoltStorage) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

// BoltSession wraps kv.Session for BoltDB backend
type BoltSession struct {
	kv.Session
}

// NewBoltSession creates a new BoltDB-backed session
func NewBoltSession(db *bbolt.DB, key string) *BoltSession {
	storage := &boltKVStorage{db: db, key: key}
	return &BoltSession{
		Session: kv.NewSession(storage, key),
	}
}

// boltKVStorage implements kv.Bucket for BoltDB
type boltKVStorage struct {
	db  *bbolt.DB
	key string
}

func (s *boltKVStorage) Set(ctx context.Context, k, v string) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(sessionBucket)
		return b.Put([]byte(k), []byte(v))
	})
}

func (s *boltKVStorage) Get(ctx context.Context, key string) (string, error) {
	var val []byte
	err := s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(sessionBucket)
		if b == nil {
			return kv.ErrKeyNotFound
		}
		val = b.Get([]byte(key))
		if val == nil {
			return kv.ErrKeyNotFound
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return string(val), nil
}
