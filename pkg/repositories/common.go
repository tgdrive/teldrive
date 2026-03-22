package repositories

import (
	"context"
	"errors"

	"github.com/go-jet/jet/v2/pgxV5"
	"github.com/go-jet/jet/v2/postgres"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// dbExecutor is the base interface for database execution (pool or transaction).
type dbExecutor interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// jetDB wraps a dbExecutor and provides Jet-specific query methods.
// This ensures all Jet operations use the same underlying executor.
type jetDB struct {
	ex dbExecutor
}

// newJetDB creates a new jetDB from a pgxpool.Pool.
func newJetDB(pool *pgxpool.Pool) jetDB {
	return jetDB{ex: pool}
}

// query executes a Jet statement and scans results into dest.
func (d jetDB) query(ctx context.Context, stmt postgres.Statement, dest any) error {
	return pgxV5.Query(ctx, stmt, d.ex, dest)
}

// exec executes a Jet statement.
func (d jetDB) exec(ctx context.Context, stmt postgres.Statement) error {
	_, err := pgxV5.Exec(ctx, stmt, d.ex)
	return normalizeDBError(err)
}

// execTag executes a Jet statement and returns the command tag.
func (d jetDB) execTag(ctx context.Context, stmt postgres.Statement) (pgconn.CommandTag, error) {
	tag, err := pgxV5.Exec(ctx, stmt, d.ex)
	return tag, normalizeDBError(err)
}

// raw returns the underlying dbExecutor executor.
func (d jetDB) raw() dbExecutor {
	return d.ex
}

// ScanRow is a helper to scan a single row.
func ScanRow(row pgx.Row, dest ...any) error {
	return row.Scan(dest...)
}

// assignmentArgs converts column assignments to interface slice for Jet.
func assignmentArgs(in []postgres.ColumnAssigment) []interface{} {
	out := make([]interface{}, len(in))
	for i := range in {
		out[i] = in[i]
	}

	return out
}

func normalizeDBError(err error) error {
	if err == nil {
		return nil
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return ErrConflict
	}

	return err
}
