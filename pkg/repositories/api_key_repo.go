package repositories

import (
	"context"
	"errors"
	"time"

	"github.com/go-jet/jet/v2/postgres"
	"github.com/go-jet/jet/v2/qrm"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tgdrive/teldrive/internal/database/jetgen/teldrive_jet/teldrive/model"
	"github.com/tgdrive/teldrive/internal/database/jetgen/teldrive_jet/teldrive/table"
)

type JetAPIKeyRepository struct {
	db jetDB
}

func NewJetAPIKeyRepository(pool *pgxpool.Pool) *JetAPIKeyRepository {
	return &JetAPIKeyRepository{db: newJetDB(pool)}
}

func (r *JetAPIKeyRepository) Create(ctx context.Context, key *model.APIKeys) error {
	now := time.Now().UTC()
	if key.ID == uuid.Nil {
		key.ID = uuid.New()
	}
	if key.CreatedAt.IsZero() {
		key.CreatedAt = now
	}
	if key.UpdatedAt.IsZero() {
		key.UpdatedAt = now
	}

	stmt := table.APIKeys.INSERT(table.APIKeys.AllColumns).MODEL(*key)
	_, err := r.db.exec(ctx, stmt)

	return err
}

func (r *JetAPIKeyRepository) ListByUserID(ctx context.Context, userID int64) ([]model.APIKeys, error) {
	stmt := table.APIKeys.
		SELECT(table.APIKeys.AllColumns).
		FROM(table.APIKeys).
		WHERE(table.APIKeys.UserID.EQ(postgres.Int64(userID)).AND(table.APIKeys.RevokedAt.IS_NULL())).
		ORDER_BY(table.APIKeys.CreatedAt.DESC())

	var out []model.APIKeys
	if err := r.db.query(ctx, stmt, &out); err != nil {
		return nil, err
	}

	return out, nil
}

func (r *JetAPIKeyRepository) GetActiveByTokenHash(ctx context.Context, tokenHash string, now time.Time) (*model.APIKeys, error) {
	stmt := table.APIKeys.
		SELECT(table.APIKeys.AllColumns).
		FROM(table.APIKeys).
		WHERE(
			table.APIKeys.TokenHash.EQ(postgres.String(tokenHash)).
				AND(table.APIKeys.RevokedAt.IS_NULL()).
				AND(table.APIKeys.ExpiresAt.IS_NULL().OR(table.APIKeys.ExpiresAt.GT(postgres.TimestampT(now)))),
		).
		LIMIT(1)

	var out model.APIKeys
	err := r.db.query(ctx, stmt, &out)
	if err != nil {
		if errors.Is(err, qrm.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return &out, nil
}

func (r *JetAPIKeyRepository) TouchLastUsed(ctx context.Context, id uuid.UUID, usedAt time.Time) error {
	stmt := table.APIKeys.UPDATE(table.APIKeys.LastUsedAt, table.APIKeys.UpdatedAt).
		SET(postgres.TimestampT(usedAt), postgres.TimestampT(usedAt)).
		WHERE(table.APIKeys.ID.EQ(postgres.UUID(id)))

	_, err := r.db.exec(ctx, stmt)

	return err
}

func (r *JetAPIKeyRepository) Revoke(ctx context.Context, userID int64, id string) error {
	idUUID, err := uuid.Parse(id)
	if err != nil {
		return ErrNotFound
	}
	now := time.Now().UTC()

	stmt := table.APIKeys.UPDATE(table.APIKeys.RevokedAt, table.APIKeys.UpdatedAt).
		SET(postgres.TimestampT(now), postgres.TimestampT(now)).
		WHERE(
			table.APIKeys.ID.EQ(postgres.UUID(idUUID)).
				AND(table.APIKeys.UserID.EQ(postgres.Int64(userID))).
				AND(table.APIKeys.RevokedAt.IS_NULL()),
		)

	res, err := r.db.exec(ctx, stmt)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}
