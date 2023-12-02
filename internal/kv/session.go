package kv

import (
	"context"
	"errors"

	"github.com/gotd/td/telegram"
)

type Session struct {
	kv  KV
	key string
}

func NewSession(kv KV, key string) telegram.SessionStorage {
	return &Session{kv: kv, key: key}
}

func (s *Session) LoadSession(_ context.Context) ([]byte, error) {

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
	return s.kv.Set(s.key, data)
}
