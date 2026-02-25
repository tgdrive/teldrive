package repositories

import (
	"context"

	"github.com/go-jet/jet/v2/pgxV5"
	"github.com/go-jet/jet/v2/postgres"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type rawDB interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type jetDB struct {
	pool *pgxpool.Pool
	tx   pgx.Tx
}

func newJetDB(pool *pgxpool.Pool) jetDB {
	return jetDB{pool: pool}
}

func (d jetDB) query(ctx context.Context, stmt postgres.Statement, dest any) error {
	if tx, ok := txFromContext(ctx); ok {
		return pgxV5.Query(ctx, stmt, tx, dest)
	}

	if d.tx != nil {
		return pgxV5.Query(ctx, stmt, d.tx, dest)
	}

	return pgxV5.Query(ctx, stmt, d.pool, dest)
}

func (d jetDB) exec(ctx context.Context, stmt postgres.Statement) (pgconn.CommandTag, error) {
	if tx, ok := txFromContext(ctx); ok {
		return pgxV5.Exec(ctx, stmt, tx)
	}

	if d.tx != nil {
		return pgxV5.Exec(ctx, stmt, d.tx)
	}

	return pgxV5.Exec(ctx, stmt, d.pool)
}

func (d jetDB) raw() rawDB {
	if d.tx != nil {
		return d.tx
	}

	return d.pool
}

func assignmentArgs(in []postgres.ColumnAssigment) []interface{} {
	out := make([]interface{}, len(in))
	for i := range in {
		out[i] = in[i]
	}

	return out
}
