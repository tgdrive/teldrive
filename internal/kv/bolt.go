package kv

import (
	"path/filepath"
	"time"

	"github.com/divyam234/teldrive/internal/config"
	"github.com/divyam234/teldrive/internal/utils"
	"go.etcd.io/bbolt"
)

type Bolt struct {
	bucket []byte
	db     *bbolt.DB
}

func (b *Bolt) Get(key string) ([]byte, error) {
	var val []byte

	if err := b.db.View(func(tx *bbolt.Tx) error {
		val = tx.Bucket(b.bucket).Get([]byte(key))
		return nil
	}); err != nil {
		return nil, err
	}

	if val == nil {
		return nil, ErrNotFound
	}
	return val, nil
}

func (b *Bolt) Set(key string, val []byte) error {
	return b.db.Update(func(tx *bbolt.Tx) error {
		return tx.Bucket(b.bucket).Put([]byte(key), val)
	})
}

func (b *Bolt) Delete(key string) error {
	return b.db.Update(func(tx *bbolt.Tx) error {
		return tx.Bucket(b.bucket).Delete([]byte(key))
	})
}

func NewBoltKV(cnf *config.Config) KV {

	sessionFile := cnf.TG.SessionFile
	if sessionFile == "" {
		sessionFile = filepath.Join(utils.ExecutableDir(), "teldrive.db")
	}
	boltDB, err := bbolt.Open(sessionFile, 0666, &bbolt.Options{
		Timeout:    time.Second,
		NoGrowSync: false,
	})
	if err != nil {
		panic(err)
	}
	kv, err := New(Options{Bucket: "teldrive", DB: boltDB})
	if err != nil {
		panic(err)
	}

	return kv
}
