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

type JetShareRepository struct {
	db jetDB
}

func NewJetShareRepository(pool *pgxpool.Pool) *JetShareRepository {
	return &JetShareRepository{db: newJetDB(pool)}
}

func (r *JetShareRepository) Create(ctx context.Context, share *model.FileShares) error {
	now := time.Now().UTC()
	if share.CreatedAt.IsZero() {
		share.CreatedAt = now
	}
	if share.UpdatedAt.IsZero() {
		share.UpdatedAt = now
	}

	stmt := table.FileShares.INSERT(table.FileShares.AllColumns).MODEL(*share)
	err := r.db.exec(ctx, stmt)

	return err
}

func (r *JetShareRepository) GetByFileID(ctx context.Context, fileID uuid.UUID) ([]model.FileShares, error) {
	stmt := table.FileShares.
		SELECT(table.FileShares.AllColumns).
		FROM(table.FileShares).
		WHERE(table.FileShares.FileID.EQ(postgres.UUID(fileID))).
		ORDER_BY(table.FileShares.CreatedAt.DESC())

	var out []model.FileShares
	if err := r.db.query(ctx, stmt, &out); err != nil {
		return nil, err
	}

	return out, nil
}

func (r *JetShareRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.FileShares, error) {
	stmt := table.FileShares.SELECT(table.FileShares.AllColumns).FROM(table.FileShares).WHERE(table.FileShares.ID.EQ(postgres.UUID(id)))

	var out model.FileShares
	if err := r.db.query(ctx, stmt, &out); err != nil {
		if errors.Is(err, qrm.ErrNoRows) {
			return nil, ErrNotFound
		}

		return nil, err
	}

	return &out, nil
}

func (r *JetShareRepository) Update(ctx context.Context, id uuid.UUID, update ShareUpdate) error {
	updates := make([]postgres.ColumnAssigment, 0, 3)

	if update.Password != nil {
		updates = append(updates, table.FileShares.Password.SET(postgres.String(*update.Password)))
	}
	if update.ExpiresAt != nil {
		updates = append(updates, table.FileShares.ExpiresAt.SET(postgres.TimestampT(*update.ExpiresAt)))
	}

	updates = append(updates, table.FileShares.UpdatedAt.SET(postgres.TimestampT(time.Now().UTC())))

	stmt := table.FileShares.UPDATE().WHERE(table.FileShares.ID.EQ(postgres.UUID(id)))
	stmt = stmt.SET(updates[0], assignmentArgs(updates[1:])...)

	err := r.db.exec(ctx, stmt)

	return err
}

func (r *JetShareRepository) Delete(ctx context.Context, id uuid.UUID) error {
	stmt := table.FileShares.DELETE().WHERE(table.FileShares.ID.EQ(postgres.UUID(id)))
	err := r.db.exec(ctx, stmt)

	return err
}

func (r *JetShareRepository) DeleteByFileID(ctx context.Context, fileID uuid.UUID) error {
	stmt := table.FileShares.DELETE().WHERE(table.FileShares.FileID.EQ(postgres.UUID(fileID)))
	err := r.db.exec(ctx, stmt)

	return err
}
