package kv

import (
	"errors"

	"go.etcd.io/bbolt"
)

var ErrNotFound = errors.New("key not found")

type KV interface {
	Get(key string) ([]byte, error)
	Set(key string, value []byte) error
	Delete(key string) error
}

type Options struct {
	Bucket string
	DB     *bbolt.DB
}

func New(opts Options) (KV, error) {

	if err := opts.DB.Update(func(tx *bbolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte(opts.Bucket))
		return err
	}); err != nil {
		return nil, err
	}

	return &Bolt{db: opts.DB, bucket: []byte(opts.Bucket)}, nil
}
