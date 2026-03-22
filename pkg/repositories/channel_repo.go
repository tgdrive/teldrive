package repositories

import (
	"context"
	"errors"
	"time"

	"github.com/go-jet/jet/v2/postgres"
	"github.com/go-jet/jet/v2/qrm"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tgdrive/teldrive/internal/database/jetgen/teldrive_jet/teldrive/model"
	"github.com/tgdrive/teldrive/internal/database/jetgen/teldrive_jet/teldrive/table"
)

type JetChannelRepository struct {
	db jetDB
}

func NewJetChannelRepository(pool *pgxpool.Pool) *JetChannelRepository {
	return &JetChannelRepository{db: newJetDB(pool)}
}

func (r *JetChannelRepository) Create(ctx context.Context, channel *model.Channels) error {
	if channel.CreatedAt.IsZero() {
		channel.CreatedAt = time.Now().UTC()
	}

	stmt := table.Channels.INSERT(table.Channels.AllColumns).MODEL(*channel)
	err := r.db.exec(ctx, stmt)

	return err
}

func (r *JetChannelRepository) GetByUserID(ctx context.Context, userID int64) ([]model.Channels, error) {
	stmt := table.Channels.
		SELECT(table.Channels.AllColumns).
		FROM(table.Channels).
		WHERE(table.Channels.UserID.EQ(postgres.Int64(userID))).
		ORDER_BY(table.Channels.ChannelID.ASC())

	var out []model.Channels
	if err := r.db.query(ctx, stmt, &out); err != nil {
		return nil, err
	}

	return out, nil
}

func (r *JetChannelRepository) GetByChannelID(ctx context.Context, channelID int64) (*model.Channels, error) {
	stmt := table.Channels.SELECT(table.Channels.AllColumns).FROM(table.Channels).WHERE(table.Channels.ChannelID.EQ(postgres.Int64(channelID)))

	var out model.Channels
	if err := r.db.query(ctx, stmt, &out); err != nil {
		if errors.Is(err, qrm.ErrNoRows) {
			return nil, ErrNotFound
		}

		return nil, err
	}

	return &out, nil
}

func (r *JetChannelRepository) GetSelected(ctx context.Context, userID int64) (*model.Channels, error) {
	stmt := table.Channels.
		SELECT(table.Channels.AllColumns).
		FROM(table.Channels).
		WHERE(
			table.Channels.UserID.EQ(postgres.Int64(userID)).
				AND(table.Channels.Selected.EQ(postgres.Bool(true))),
		)

	var out model.Channels
	if err := r.db.query(ctx, stmt, &out); err != nil {
		if errors.Is(err, qrm.ErrNoRows) {
			return nil, ErrNotFound
		}

		return nil, err
	}

	return &out, nil
}

func (r *JetChannelRepository) Update(ctx context.Context, channelID int64, update ChannelUpdate) error {
	updates := make([]postgres.ColumnAssigment, 0, 2)

	if update.ChannelName != nil {
		updates = append(updates, table.Channels.ChannelName.SET(postgres.String(*update.ChannelName)))
	}
	if update.Selected != nil {
		updates = append(updates, table.Channels.Selected.SET(postgres.Bool(*update.Selected)))
	}

	if len(updates) == 0 {
		return nil
	}

	stmt := table.Channels.UPDATE().WHERE(table.Channels.ChannelID.EQ(postgres.Int64(channelID)))
	stmt = stmt.SET(updates[0], assignmentArgs(updates[1:])...)

	err := r.db.exec(ctx, stmt)

	return err
}

func (r *JetChannelRepository) Delete(ctx context.Context, channelID int64) error {
	stmt := table.Channels.DELETE().WHERE(table.Channels.ChannelID.EQ(postgres.Int64(channelID)))
	err := r.db.exec(ctx, stmt)

	return err
}

func (r *JetChannelRepository) DeleteByUserID(ctx context.Context, userID int64) error {
	stmt := table.Channels.DELETE().WHERE(table.Channels.UserID.EQ(postgres.Int64(userID)))
	err := r.db.exec(ctx, stmt)

	return err
}
