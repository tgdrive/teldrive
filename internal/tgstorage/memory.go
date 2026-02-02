package tgstorage

import (
	"context"

	"github.com/gotd/td/session"
)

var _ Storage = (*MemoryStorage)(nil)

// MemoryStorage implements session storage using in-memory storage
// This is suitable for development/testing only - data is lost on restart
type MemoryStorage struct {
	storage *session.StorageMemory
}

// NewMemoryStorage creates a new in-memory session storage
func NewMemoryStorage() *MemoryStorage {
	return &MemoryStorage{
		storage: new(session.StorageMemory),
	}
}

// LoadSession retrieves session data from memory
func (s *MemoryStorage) LoadSession(ctx context.Context) ([]byte, error) {
	return s.storage.LoadSession(ctx)
}

// StoreSession saves session data to memory
func (s *MemoryStorage) StoreSession(ctx context.Context, data []byte) error {
	return s.storage.StoreSession(ctx, data)
}

// Type returns the storage type
func (s *MemoryStorage) Type() string {
	return "memory"
}

// Close is a no-op for memory storage
func (s *MemoryStorage) Close() error {
	return nil
}
