package kv

import (
	"context"
	"errors"
	"sync"

	"github.com/gotd/td/telegram"
)

type Session struct {
	kv  KV
	key string
	mu  sync.Mutex
}

func NewSession(kv KV, key string) telegram.SessionStorage {
	return &Session{kv: kv, key: key}
}

func (s *Session) LoadSession(_ context.Context) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	b, err := s.kv.Get(s.key)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return b, nil
}

func (s *Session) StoreSession(_ context.Context, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.kv.Set(s.key, data)
}
