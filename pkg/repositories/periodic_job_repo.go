package repositories

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/go-jet/jet/v2/postgres"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tgdrive/teldrive/internal/database/jet/gen/model"
	"github.com/tgdrive/teldrive/internal/database/jet/gen/table"
	"github.com/tgdrive/teldrive/internal/database/types"
)

type JetPeriodicJobRepository struct {
	db jetDB
}

func NewJetPeriodicJobRepository(pool *pgxpool.Pool) *JetPeriodicJobRepository {
	return &JetPeriodicJobRepository{db: newJetDB(pool)}
}

func (r *JetPeriodicJobRepository) Create(ctx context.Context, job *PeriodicJob) error {
	argsJSON, err := encodePeriodicJobArgs(job.Kind, job.Args)
	if err != nil {
		return err
	}

	stmt := table.PeriodicJobs.INSERT(
		table.PeriodicJobs.ID,
		table.PeriodicJobs.UserID,
		table.PeriodicJobs.Name,
		table.PeriodicJobs.Kind,
		table.PeriodicJobs.Args,
		table.PeriodicJobs.CronExpression,
		table.PeriodicJobs.Enabled,
		table.PeriodicJobs.System,
		table.PeriodicJobs.CreatedAt,
		table.PeriodicJobs.UpdatedAt,
	).VALUES(
		postgres.UUID(job.ID),
		postgres.Int64(job.UserID),
		postgres.String(job.Name),
		postgres.String(job.Kind),
		postgres.StringExp(postgres.CAST(postgres.String(argsJSON)).AS("jsonb")),
		postgres.String(job.CronExpression),
		postgres.Bool(job.Enabled),
		postgres.Bool(job.System),
		postgres.TimestampzT(job.CreatedAt),
		postgres.TimestampzT(job.UpdatedAt),
	)
	err = r.db.exec(ctx, stmt)
	return err
}

func (r *JetPeriodicJobRepository) ListByUserID(ctx context.Context, userID int64) ([]PeriodicJob, error) {
	stmt := table.PeriodicJobs.
		SELECT(
			table.PeriodicJobs.ID,
			table.PeriodicJobs.UserID,
			table.PeriodicJobs.Name,
			table.PeriodicJobs.Kind,
			table.PeriodicJobs.Args,
			table.PeriodicJobs.CronExpression,
			table.PeriodicJobs.Enabled,
			table.PeriodicJobs.System,
			table.PeriodicJobs.CreatedAt,
			table.PeriodicJobs.UpdatedAt,
		).
		FROM(table.PeriodicJobs).
		WHERE(table.PeriodicJobs.UserID.EQ(postgres.Int64(userID))).
		ORDER_BY(table.PeriodicJobs.System.DESC(), table.PeriodicJobs.CreatedAt.ASC())

	query, args := stmt.Sql()
	rows, err := r.db.raw().Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PeriodicJob
	for rows.Next() {
		item, scanErr := scanPeriodicJob(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *JetPeriodicJobRepository) ListEnabled(ctx context.Context) ([]PeriodicJob, error) {
	stmt := table.PeriodicJobs.
		SELECT(
			table.PeriodicJobs.ID,
			table.PeriodicJobs.UserID,
			table.PeriodicJobs.Name,
			table.PeriodicJobs.Kind,
			table.PeriodicJobs.Args,
			table.PeriodicJobs.CronExpression,
			table.PeriodicJobs.Enabled,
			table.PeriodicJobs.System,
			table.PeriodicJobs.CreatedAt,
			table.PeriodicJobs.UpdatedAt,
		).
		FROM(table.PeriodicJobs).
		WHERE(table.PeriodicJobs.Enabled.EQ(postgres.Bool(true))).
		ORDER_BY(table.PeriodicJobs.UserID.ASC(), table.PeriodicJobs.CreatedAt.ASC())

	query, args := stmt.Sql()
	rows, err := r.db.raw().Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PeriodicJob
	for rows.Next() {
		item, scanErr := scanPeriodicJob(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *JetPeriodicJobRepository) GetByIDAndUserID(ctx context.Context, id uuid.UUID, userID int64) (*PeriodicJob, error) {
	stmt := table.PeriodicJobs.
		SELECT(
			table.PeriodicJobs.ID,
			table.PeriodicJobs.UserID,
			table.PeriodicJobs.Name,
			table.PeriodicJobs.Kind,
			table.PeriodicJobs.Args,
			table.PeriodicJobs.CronExpression,
			table.PeriodicJobs.Enabled,
			table.PeriodicJobs.System,
			table.PeriodicJobs.CreatedAt,
			table.PeriodicJobs.UpdatedAt,
		).
		FROM(table.PeriodicJobs).
		WHERE(table.PeriodicJobs.ID.EQ(postgres.UUID(id)).AND(table.PeriodicJobs.UserID.EQ(postgres.Int64(userID))))

	query, args := stmt.Sql()
	row := r.db.raw().QueryRow(ctx, query, args...)
	out, err := scanPeriodicJob(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &out, nil
}

func (r *JetPeriodicJobRepository) GetByNameAndUserID(ctx context.Context, userID int64, name string) (*PeriodicJob, error) {
	stmt := table.PeriodicJobs.
		SELECT(
			table.PeriodicJobs.ID,
			table.PeriodicJobs.UserID,
			table.PeriodicJobs.Name,
			table.PeriodicJobs.Kind,
			table.PeriodicJobs.Args,
			table.PeriodicJobs.CronExpression,
			table.PeriodicJobs.Enabled,
			table.PeriodicJobs.System,
			table.PeriodicJobs.CreatedAt,
			table.PeriodicJobs.UpdatedAt,
		).
		FROM(table.PeriodicJobs).
		WHERE(table.PeriodicJobs.UserID.EQ(postgres.Int64(userID)).AND(table.PeriodicJobs.Name.EQ(postgres.String(name))))

	query, args := stmt.Sql()
	row := r.db.raw().QueryRow(ctx, query, args...)
	out, err := scanPeriodicJob(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &out, nil
}

func (r *JetPeriodicJobRepository) Update(ctx context.Context, id uuid.UUID, userID int64, job PeriodicJob) error {
	argsJSON, err := encodePeriodicJobArgs(job.Kind, job.Args)
	if err != nil {
		return err
	}

	stmt := table.PeriodicJobs.UPDATE(
		table.PeriodicJobs.Name,
		table.PeriodicJobs.CronExpression,
		table.PeriodicJobs.Enabled,
		table.PeriodicJobs.Args,
		table.PeriodicJobs.UpdatedAt,
	).SET(
		table.PeriodicJobs.Name.SET(postgres.String(job.Name)),
		table.PeriodicJobs.CronExpression.SET(postgres.String(job.CronExpression)),
		table.PeriodicJobs.Enabled.SET(postgres.Bool(job.Enabled)),
		table.PeriodicJobs.Args.SET(postgres.StringExp(postgres.CAST(postgres.String(argsJSON)).AS("jsonb"))),
		table.PeriodicJobs.UpdatedAt.SET(postgres.TimestampzT(job.UpdatedAt)),
	).WHERE(
		table.PeriodicJobs.ID.EQ(postgres.UUID(id)).AND(table.PeriodicJobs.UserID.EQ(postgres.Int64(userID))),
	)
	err = r.db.exec(ctx, stmt)
	return err
}

func (r *JetPeriodicJobRepository) Delete(ctx context.Context, id uuid.UUID, userID int64) error {
	stmt := table.PeriodicJobs.DELETE().WHERE(
		table.PeriodicJobs.ID.EQ(postgres.UUID(id)).AND(table.PeriodicJobs.UserID.EQ(postgres.Int64(userID))),
	)
	err := r.db.exec(ctx, stmt)
	return err
}

func (r *JetPeriodicJobRepository) SetEnabled(ctx context.Context, id uuid.UUID, userID int64, enabled bool, updatedAt time.Time) error {
	stmt := table.PeriodicJobs.UPDATE(table.PeriodicJobs.Enabled, table.PeriodicJobs.UpdatedAt).
		MODEL(model.PeriodicJobs{Enabled: enabled, UpdatedAt: updatedAt}).
		WHERE(table.PeriodicJobs.ID.EQ(postgres.UUID(id)).AND(table.PeriodicJobs.UserID.EQ(postgres.Int64(userID))))
	err := r.db.exec(ctx, stmt)
	return err
}
func scanPeriodicJob(row interface{ Scan(dest ...any) error }) (PeriodicJob, error) {
	var out PeriodicJob
	var argsRaw types.JSONB[json.RawMessage]
	if err := row.Scan(
		&out.ID,
		&out.UserID,
		&out.Name,
		&out.Kind,
		&argsRaw,
		&out.CronExpression,
		&out.Enabled,
		&out.System,
		&out.CreatedAt,
		&out.UpdatedAt,
	); err != nil {
		return PeriodicJob{}, err
	}
	args, err := decodePeriodicJobArgs(out.Kind, argsRaw.Data)
	if err != nil {
		return PeriodicJob{}, err
	}
	out.Args = args
	return out, nil
}

func encodePeriodicJobArgs(kind string, args PeriodicJobArgs) (string, error) {
	if args == nil {
		return "{}", nil
	}

	switch kind {
	case "sync.run":
		if _, ok := args.(SyncRunPeriodicArgs); !ok {
			if _, ok := args.(*SyncRunPeriodicArgs); !ok {
				return "", fmt.Errorf("invalid args type for kind %s", kind)
			}
		}
	case "clean.old_events":
		if _, ok := args.(CleanOldEventsPeriodicArgs); !ok {
			if _, ok := args.(*CleanOldEventsPeriodicArgs); !ok {
				return "", fmt.Errorf("invalid args type for kind %s", kind)
			}
		}
	case "clean.stale_uploads":
		if _, ok := args.(CleanStaleUploadsPeriodicArgs); !ok {
			if _, ok := args.(*CleanStaleUploadsPeriodicArgs); !ok {
				return "", fmt.Errorf("invalid args type for kind %s", kind)
			}
		}
	case "clean.pending_files":
		if _, ok := args.(CleanPendingFilesPeriodicArgs); !ok {
			if _, ok := args.(*CleanPendingFilesPeriodicArgs); !ok {
				return "", fmt.Errorf("invalid args type for kind %s", kind)
			}
		}
	default:
		return "", fmt.Errorf("unsupported periodic job kind: %s", kind)
	}

	b, err := json.Marshal(args)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func decodePeriodicJobArgs(kind string, raw json.RawMessage) (PeriodicJobArgs, error) {
	if len(raw) == 0 || string(raw) == "null" {
		raw = []byte("{}")
	}

	switch kind {
	case "sync.run":
		var out SyncRunPeriodicArgs
		if err := json.Unmarshal(raw, &out); err != nil {
			return nil, err
		}
		return out, nil
	case "clean.old_events":
		var out CleanOldEventsPeriodicArgs
		if err := json.Unmarshal(raw, &out); err != nil {
			return nil, err
		}
		return out, nil
	case "clean.stale_uploads":
		var out CleanStaleUploadsPeriodicArgs
		if err := json.Unmarshal(raw, &out); err != nil {
			return nil, err
		}
		return out, nil
	case "clean.pending_files":
		var out CleanPendingFilesPeriodicArgs
		if err := json.Unmarshal(raw, &out); err != nil {
			return nil, err
		}
		return out, nil
	default:
		return nil, fmt.Errorf("unsupported periodic job kind: %s", kind)
	}
}
