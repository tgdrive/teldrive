package repositories

import (
	"context"

	"github.com/go-jet/jet/v2/postgres"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tgdrive/teldrive/internal/database/jet/gen/model"
	"github.com/tgdrive/teldrive/internal/database/jet/gen/table"
)

type JetBotRepository struct {
	db jetDB
}

func NewJetBotRepository(pool *pgxpool.Pool) *JetBotRepository {
	return &JetBotRepository{db: newJetDB(pool)}
}

func (r *JetBotRepository) CreateToken(ctx context.Context, userID int64, token string) error {
	return r.Create(ctx, &model.Bots{UserID: userID, Token: token})
}

func (r *JetBotRepository) Create(ctx context.Context, bot *model.Bots) error {
	stmt := table.Bots.INSERT(table.Bots.AllColumns).MODEL(*bot)
	err := r.db.exec(ctx, stmt)

	return err
}

func (r *JetBotRepository) GetByUserID(ctx context.Context, userID int64) ([]model.Bots, error) {
	stmt := table.Bots.SELECT(table.Bots.AllColumns).FROM(table.Bots).WHERE(table.Bots.UserID.EQ(postgres.Int64(userID)))

	var out []model.Bots
	if err := r.db.query(ctx, stmt, &out); err != nil {
		return nil, err
	}

	return out, nil
}

func (r *JetBotRepository) GetTokensByUserID(ctx context.Context, userID int64) ([]string, error) {
	stmt := table.Bots.
		SELECT(table.Bots.Token).
		FROM(table.Bots).
		WHERE(table.Bots.UserID.EQ(postgres.Int64(userID))).
		ORDER_BY(table.Bots.Token.ASC())

	var rows []struct{ Token string }
	if err := r.db.query(ctx, stmt, &rows); err != nil {
		return nil, err
	}

	tokens := make([]string, 0, len(rows))
	for _, row := range rows {
		tokens = append(tokens, row.Token)
	}

	return tokens, nil
}

func (r *JetBotRepository) Delete(ctx context.Context, userID int64, token string) error {
	stmt := table.Bots.DELETE().WHERE(
		table.Bots.UserID.EQ(postgres.Int64(userID)).
			AND(table.Bots.Token.EQ(postgres.String(token))),
	)

	err := r.db.exec(ctx, stmt)

	return err
}

func (r *JetBotRepository) DeleteByUserID(ctx context.Context, userID int64) error {
	stmt := table.Bots.DELETE().WHERE(table.Bots.UserID.EQ(postgres.Int64(userID)))
	err := r.db.exec(ctx, stmt)

	return err
}
