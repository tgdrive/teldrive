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

type JetUploadRepository struct {
	db jetDB
}

func NewJetUploadRepository(pool *pgxpool.Pool) *JetUploadRepository {
	return &JetUploadRepository{db: newJetDB(pool)}
}

func (r *JetUploadRepository) Create(ctx context.Context, upload *model.Uploads) error {
	if upload.CreatedAt == nil {
		now := time.Now().UTC()
		upload.CreatedAt = &now
	}

	stmt := table.Uploads.INSERT(table.Uploads.AllColumns).MODEL(*upload)
	err := r.db.exec(ctx, stmt)

	return err
}

func (r *JetUploadRepository) GetByUploadID(ctx context.Context, uploadID string) ([]model.Uploads, error) {
	stmt := table.Uploads.
		SELECT(table.Uploads.AllColumns).
		FROM(table.Uploads).
		WHERE(table.Uploads.UploadID.EQ(postgres.String(uploadID))).
		ORDER_BY(table.Uploads.PartNo.ASC())

	var out []model.Uploads
	if err := r.db.query(ctx, stmt, &out); err != nil {
		return nil, err
	}

	return out, nil
}

func (r *JetUploadRepository) GetByUploadIDAndRetention(ctx context.Context, uploadID string, retention time.Duration) ([]model.Uploads, error) {
	threshold := time.Now().UTC().Add(-retention)
	stmt := table.Uploads.
		SELECT(table.Uploads.AllColumns).
		FROM(table.Uploads).
		WHERE(
			table.Uploads.UploadID.EQ(postgres.String(uploadID)).
				AND(table.Uploads.CreatedAt.LT(postgres.TimestampT(threshold))),
		).
		ORDER_BY(table.Uploads.PartNo.ASC())

	var out []model.Uploads
	if err := r.db.query(ctx, stmt, &out); err != nil {
		return nil, err
	}

	return out, nil
}

func (r *JetUploadRepository) Delete(ctx context.Context, uploadID string) error {
	stmt := table.Uploads.DELETE().WHERE(table.Uploads.UploadID.EQ(postgres.String(uploadID)))
	err := r.db.exec(ctx, stmt)

	return err
}

func (r *JetUploadRepository) ListStale(ctx context.Context, before time.Time) ([]StaleUpload, error) {
	stmt := table.Uploads.
		SELECT(table.Uploads.PartID, table.Uploads.ChannelID, table.Uploads.UserID).
		FROM(table.Uploads).
		WHERE(table.Uploads.CreatedAt.LT(postgres.TimestampT(before)))

	var out []StaleUpload
	if err := r.db.query(ctx, stmt, &out); err != nil {
		if errors.Is(err, qrm.ErrNoRows) {
			return []StaleUpload{}, nil
		}
		return nil, err
	}

	return out, nil
}

func (r *JetUploadRepository) DeleteParts(ctx context.Context, channelID, userID int64, partIDs []int) error {
	if len(partIDs) == 0 {
		return nil
	}

	partExprs := make([]postgres.Expression, 0, len(partIDs))
	for _, partID := range partIDs {
		partExprs = append(partExprs, postgres.Int(int64(partID)))
	}

	stmt := table.Uploads.DELETE().WHERE(
		table.Uploads.ChannelID.EQ(postgres.Int64(channelID)).
			AND(table.Uploads.UserID.EQ(postgres.Int64(userID))).
			AND(table.Uploads.PartID.IN(partExprs...)),
	)

	return r.db.exec(ctx, stmt)
}

func (r *JetUploadRepository) DeleteOlderThan(ctx context.Context, before time.Time) (int64, error) {
	stmt := table.Uploads.DELETE().WHERE(table.Uploads.CreatedAt.LT(postgres.TimestampT(before)))
	tag, err := r.db.execTag(ctx, stmt)
	if err != nil {
		return 0, err
	}

	return tag.RowsAffected(), nil
}

func (r *JetUploadRepository) ListPartIDsByChannel(ctx context.Context, userID, channelID int64) ([]int, error) {
	stmt := table.Uploads.
		SELECT(table.Uploads.PartID).
		FROM(table.Uploads).
		WHERE(table.Uploads.UserID.EQ(postgres.Int64(userID)).AND(table.Uploads.ChannelID.EQ(postgres.Int64(channelID))))

	query, args := stmt.Sql()
	rows, err := r.db.raw().Query(ctx, query, args...)
	if err != nil {
		return nil, normalizeDBError(err)
	}
	defer rows.Close()

	out := []int{}
	for rows.Next() {
		var id int32
		if err := rows.Scan(&id); err != nil {
			return nil, normalizeDBError(err)
		}
		out = append(out, int(id))
	}
	if err := rows.Err(); err != nil {
		return nil, normalizeDBError(err)
	}
	return out, nil
}

func (r *JetUploadRepository) DeleteByID(ctx context.Context, partID int32, channelID int64) error {
	stmt := table.Uploads.DELETE().WHERE(
		table.Uploads.PartID.EQ(postgres.Int32(partID)).
			AND(table.Uploads.ChannelID.EQ(postgres.Int64(channelID))),
	)

	err := r.db.exec(ctx, stmt)

	return err
}

func (r *JetUploadRepository) StatsByDays(ctx context.Context, userID int64, days int) ([]UploadStat, error) {
	if days <= 0 {
		days = 1
	}

	startDay := time.Now().UTC().Truncate(24*time.Hour).AddDate(0, 0, -(days - 1))
	endDay := startDay.AddDate(0, 0, days)

	stmt := table.Files.SELECT(table.Files.CreatedAt, table.Files.Size).FROM(table.Files).WHERE(
		table.Files.UserID.EQ(postgres.Int64(userID)).
			AND(table.Files.Type.EQ(postgres.String("file"))).
			AND(table.Files.CreatedAt.GT_EQ(postgres.TimestampT(startDay))).
			AND(table.Files.CreatedAt.LT(postgres.TimestampT(endDay))),
	)

	var rows []struct {
		CreatedAt time.Time
		Size      *int64
	}
	if err := r.db.query(ctx, stmt, &rows); err != nil {
		if errors.Is(err, qrm.ErrNoRows) {
			rows = nil
		} else {
			return nil, err
		}
	}

	totals := make(map[time.Time]int64, days)
	for _, row := range rows {
		day := row.CreatedAt.UTC().Truncate(24 * time.Hour)
		if row.Size != nil {
			totals[day] += *row.Size
		}
	}

	out := make([]UploadStat, 0, days)
	for i := 0; i < days; i++ {
		day := startDay.AddDate(0, 0, i)
		out = append(out, UploadStat{UploadDate: day, TotalUploaded: totals[day]})
	}

	return out, nil
}
