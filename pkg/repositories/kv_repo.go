package repositories

import (
	"context"
	"errors"
	"time"

	"github.com/go-jet/jet/v2/postgres"
	"github.com/go-jet/jet/v2/qrm"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tgdrive/teldrive/internal/database/jet/gen/model"
	"github.com/tgdrive/teldrive/internal/database/jet/gen/table"
)

type JetKVRepository struct {
	db jetDB
}

func NewJetKVRepository(pool *pgxpool.Pool) *JetKVRepository {
	return &JetKVRepository{db: newJetDB(pool)}
}

func (r *JetKVRepository) Set(ctx context.Context, item *model.Kv) error {
	if item.CreatedAt.IsZero() {
		item.CreatedAt = time.Now().UTC()
	}

	stmt := table.Kv.
		INSERT(table.Kv.AllColumns).
		MODEL(*item).
		ON_CONFLICT(table.Kv.Key).
		DO_UPDATE(postgres.SET(
			table.Kv.Value.SET(table.Kv.EXCLUDED.Value),
			table.Kv.CreatedAt.SET(table.Kv.EXCLUDED.CreatedAt),
		))

	err := r.db.exec(ctx, stmt)

	return err
}

func (r *JetKVRepository) Get(ctx context.Context, key string) (*model.Kv, error) {
	stmt := table.Kv.SELECT(table.Kv.AllColumns).FROM(table.Kv).WHERE(table.Kv.Key.EQ(postgres.String(key)))

	var out model.Kv
	if err := r.db.query(ctx, stmt, &out); err != nil {
		if errors.Is(err, qrm.ErrNoRows) {
			return nil, ErrNotFound
		}

		return nil, err
	}

	return &out, nil
}

func (r *JetKVRepository) Delete(ctx context.Context, key string) error {
	stmt := table.Kv.DELETE().WHERE(table.Kv.Key.EQ(postgres.String(key)))
	err := r.db.exec(ctx, stmt)

	return err
}

func (r *JetKVRepository) DeletePrefix(ctx context.Context, prefix string) error {
	stmt := table.Kv.DELETE().WHERE(table.Kv.Key.LIKE(postgres.String(prefix + "%")))
	err := r.db.exec(ctx, stmt)

	return err
}

func (r *JetKVRepository) Iterate(ctx context.Context, prefix string, fn func(key string, value []byte) error) error {
	stmt := table.Kv.
		SELECT(table.Kv.AllColumns).
		FROM(table.Kv).
		WHERE(table.Kv.Key.LIKE(postgres.String(prefix + "%"))).
		ORDER_BY(table.Kv.Key.ASC())

	var entries []model.Kv
	if err := r.db.query(ctx, stmt, &entries); err != nil {
		return err
	}

	for _, entry := range entries {
		if err := fn(entry.Key, entry.Value); err != nil {
			return err
		}
	}

	return nil
}
