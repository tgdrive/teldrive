package repositories

import (
	"context"
	"time"

	"github.com/go-jet/jet/v2/postgres"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tgdrive/teldrive/internal/database/jetgen/teldrive_jet/teldrive/model"
	"github.com/tgdrive/teldrive/internal/database/jetgen/teldrive_jet/teldrive/table"
)

type JetEventRepository struct {
	db jetDB
}

func NewJetEventRepository(pool *pgxpool.Pool) *JetEventRepository {
	return &JetEventRepository{db: newJetDB(pool)}
}

func (r *JetEventRepository) Create(ctx context.Context, event *model.Events) error {
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}

	stmt := table.Events.INSERT(table.Events.AllColumns).MODEL(*event)
	err := r.db.exec(ctx, stmt)

	return err
}

func (r *JetEventRepository) GetByUserID(ctx context.Context, userID int64, since time.Time) ([]model.Events, error) {
	stmt := table.Events.
		SELECT(table.Events.AllColumns).
		FROM(table.Events).
		WHERE(
			table.Events.UserID.EQ(postgres.Int64(userID)).
				AND(table.Events.CreatedAt.GT(postgres.TimestampT(since))),
		).
		ORDER_BY(table.Events.CreatedAt.DESC())

	var out []model.Events
	if err := r.db.query(ctx, stmt, &out); err != nil {
		return nil, err
	}

	return out, nil
}

func (r *JetEventRepository) GetRecent(ctx context.Context, userID int64, since time.Time, limit int) ([]model.Events, error) {
	stmt := table.Events.
		SELECT(table.Events.AllColumns).
		FROM(table.Events).
		WHERE(
			table.Events.UserID.EQ(postgres.Int64(userID)).
				AND(table.Events.CreatedAt.GT(postgres.TimestampT(since))),
		).
		ORDER_BY(table.Events.CreatedAt.DESC())

	if limit > 0 {
		stmt = stmt.LIMIT(int64(limit))
	}

	var out []model.Events
	if err := r.db.query(ctx, stmt, &out); err != nil {
		return nil, err
	}

	return out, nil
}

func (r *JetEventRepository) GetSince(ctx context.Context, since time.Time, limit int) ([]model.Events, error) {
	stmt := table.Events.
		SELECT(table.Events.AllColumns).
		FROM(table.Events).
		WHERE(table.Events.CreatedAt.GT(postgres.TimestampT(since))).
		ORDER_BY(table.Events.CreatedAt.ASC())

	if limit > 0 {
		stmt = stmt.LIMIT(int64(limit))
	}

	var out []model.Events
	if err := r.db.query(ctx, stmt, &out); err != nil {
		return nil, err
	}

	return out, nil
}

func (r *JetEventRepository) DeleteOlderThan(ctx context.Context, before time.Time) (int64, error) {
	stmt := table.Events.DELETE().WHERE(table.Events.CreatedAt.LT(postgres.TimestampT(before)))

	tag, err := r.db.execTag(ctx, stmt)
	if err != nil {
		return 0, err
	}

	return tag.RowsAffected(), nil
}

func (r *JetEventRepository) DeleteOlderThanForUser(ctx context.Context, userID int64, before time.Time) (int64, error) {
	stmt := table.Events.DELETE().WHERE(
		table.Events.UserID.EQ(postgres.Int64(userID)).
			AND(table.Events.CreatedAt.LT(postgres.TimestampT(before))),
	)

	tag, err := r.db.execTag(ctx, stmt)
	if err != nil {
		return 0, err
	}

	return tag.RowsAffected(), nil
}
