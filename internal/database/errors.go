package database

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

var (
	ErrNotFound    = errors.New("record not found")
	ErrKeyConflict = errors.New("key conflict")
)

func IsRecordNotFoundErr(err error) bool {
	return err == ErrNotFound
}

func IsKeyConflictErr(err error) bool {
	if err == ErrKeyConflict {
		return true
	}
	switch e := err.(type) {
	case *pgconn.PgError:
		if e.Code == "23505" {
			return true
		}
	}
	return false
}
