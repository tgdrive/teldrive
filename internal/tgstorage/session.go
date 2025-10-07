package tgstorage

import (
	"github.com/gotd/td/session"
	"github.com/tgdrive/teldrive/internal/cache"
	"gorm.io/gorm"

	"github.com/gotd/contrib/auth/kv"
)

var _ session.Storage = SessionStorage{}

type SessionStorage struct {
	kv.Session
}

func NewSessionStorage(db *gorm.DB, cache cache.Cacher, key string) SessionStorage {
	s := &kvStorage{
		db:    db,
		cache: cache,
	}
	return SessionStorage{
		Session: kv.NewSession(s, key),
	}
}
