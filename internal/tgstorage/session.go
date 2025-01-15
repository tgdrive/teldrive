package tgstorage

import (
	"github.com/gotd/td/session"
	"gorm.io/gorm"

	"github.com/gotd/contrib/auth/kv"
)

var _ session.Storage = SessionStorage{}

type SessionStorage struct {
	kv.Session
}

func NewSessionStorage(db *gorm.DB, key string) SessionStorage {
	s := &kvStorage{
		db: db,
	}
	return SessionStorage{
		Session: kv.NewSession(s, key),
	}
}
