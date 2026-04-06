package repositories

import (
	"context"

	"github.com/jackc/pgx/v5"
)

type txContextKey struct{}

func contextWithTx(ctx context.Context, tx pgx.Tx) context.Context {
	return context.WithValue(ctx, txContextKey{}, tx)
}

func txFromContext(ctx context.Context) (pgx.Tx, bool) {
	tx, ok := ctx.Value(txContextKey{}).(pgx.Tx)
	return tx, ok
}
