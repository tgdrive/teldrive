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

type JetSessionRepository struct {
	db jetDB
}

func NewJetSessionRepository(pool *pgxpool.Pool) *JetSessionRepository {
	return &JetSessionRepository{db: newJetDB(pool)}
}

func (r *JetSessionRepository) Create(ctx context.Context, session *model.Sessions) error {
	if session.CreatedAt.IsZero() {
		session.CreatedAt = time.Now().UTC()
	}
	if session.UpdatedAt.IsZero() {
		session.UpdatedAt = time.Now().UTC()
	}
	if session.ID == uuid.Nil {
		session.ID = uuid.New()
	}

	stmt := table.Sessions.INSERT(table.Sessions.AllColumns).MODEL(*session)
	_, err := r.db.exec(ctx, stmt)

	return err
}

func (r *JetSessionRepository) GetByID(ctx context.Context, id string) (*model.Sessions, error) {
	idUUID, err := uuid.Parse(id)
	if err != nil {
		return nil, ErrNotFound
	}
	stmt := table.Sessions.SELECT(table.Sessions.AllColumns).FROM(table.Sessions).WHERE(
		table.Sessions.ID.EQ(postgres.UUID(idUUID)).AND(table.Sessions.RevokedAt.IS_NULL()),
	)

	var out model.Sessions
	if err := r.db.query(ctx, stmt, &out); err != nil {
		if errors.Is(err, qrm.ErrNoRows) {
			return nil, ErrNotFound
		}

		return nil, err
	}

	return &out, nil
}

func (r *JetSessionRepository) GetByRefreshTokenHash(ctx context.Context, refreshTokenHash string) (*model.Sessions, error) {
	stmt := table.Sessions.SELECT(table.Sessions.AllColumns).FROM(table.Sessions).WHERE(
		table.Sessions.RefreshTokenHash.EQ(postgres.String(refreshTokenHash)).AND(table.Sessions.RevokedAt.IS_NULL()),
	)

	var out model.Sessions
	if err := r.db.query(ctx, stmt, &out); err != nil {
		if errors.Is(err, qrm.ErrNoRows) {
			return nil, ErrNotFound
		}

		return nil, err
	}

	return &out, nil
}

func (r *JetSessionRepository) GetByUserID(ctx context.Context, userID int64) ([]model.Sessions, error) {
	stmt := table.Sessions.
		SELECT(table.Sessions.AllColumns).
		FROM(table.Sessions).
		WHERE(table.Sessions.UserID.EQ(postgres.Int64(userID)).AND(table.Sessions.RevokedAt.IS_NULL())).
		ORDER_BY(table.Sessions.CreatedAt.DESC())

	var out []model.Sessions
	if err := r.db.query(ctx, stmt, &out); err != nil {
		return nil, err
	}

	return out, nil
}

func (r *JetSessionRepository) UpdateRefreshTokenHash(ctx context.Context, id string, refreshTokenHash string) error {
	idUUID, err := uuid.Parse(id)
	if err != nil {
		return ErrNotFound
	}
	now := time.Now().UTC()
	stmt := table.Sessions.UPDATE(table.Sessions.RefreshTokenHash, table.Sessions.UpdatedAt).
		SET(postgres.String(refreshTokenHash), postgres.TimestampT(now)).
		WHERE(table.Sessions.ID.EQ(postgres.UUID(idUUID)).AND(table.Sessions.RevokedAt.IS_NULL()))
	_, err = r.db.exec(ctx, stmt)

	return err
}

func (r *JetSessionRepository) Revoke(ctx context.Context, id string) error {
	idUUID, err := uuid.Parse(id)
	if err != nil {
		return ErrNotFound
	}
	now := time.Now().UTC()
	stmt := table.Sessions.UPDATE(table.Sessions.RevokedAt, table.Sessions.UpdatedAt, table.Sessions.RefreshTokenHash).
		SET(postgres.TimestampT(now), postgres.TimestampT(now), postgres.NULL).
		WHERE(table.Sessions.ID.EQ(postgres.UUID(idUUID)).AND(table.Sessions.RevokedAt.IS_NULL()))
	_, err = r.db.exec(ctx, stmt)

	return err
}

func (r *JetSessionRepository) DeleteByUserID(ctx context.Context, userID int64) error {
	now := time.Now().UTC()
	stmt := table.Sessions.UPDATE(table.Sessions.RevokedAt, table.Sessions.UpdatedAt, table.Sessions.RefreshTokenHash).
		SET(postgres.TimestampT(now), postgres.TimestampT(now), postgres.NULL).
		WHERE(table.Sessions.UserID.EQ(postgres.Int64(userID)).AND(table.Sessions.RevokedAt.IS_NULL()))
	_, err := r.db.exec(ctx, stmt)

	return err
}
