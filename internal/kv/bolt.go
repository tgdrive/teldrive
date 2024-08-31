package kv

import (
	"os"
	"path/filepath"
	"time"

	"github.com/mitchellh/go-homedir"
	"github.com/tgdrive/teldrive/internal/config"
	"github.com/tgdrive/teldrive/internal/utils"
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
		dir, err := homedir.Dir()
		if err != nil {
			dir = utils.ExecutableDir()
		} else {
			dir = filepath.Join(dir, ".teldrive")
			err := os.Mkdir(dir, 0755)
			if err != nil && !os.IsExist(err) {
				dir = utils.ExecutableDir()
			}
		}
		sessionFile = filepath.Join(dir, "session.db")
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
