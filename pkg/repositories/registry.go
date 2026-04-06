package repositories

import "context"
import "github.com/jackc/pgx/v5/pgxpool"

func NewRepositories(pool *pgxpool.Pool) *Repositories {
	return &Repositories{
		Pool:         pool,
		Files:        NewJetFileRepository(pool),
		Sessions:     NewJetSessionRepository(pool),
		APIKeys:      NewJetAPIKeyRepository(pool),
		Uploads:      NewJetUploadRepository(pool),
		Channels:     NewJetChannelRepository(pool),
		Bots:         NewJetBotRepository(pool),
		Users:        NewJetUserRepository(pool),
		Shares:       NewJetShareRepository(pool),
		Events:       NewJetEventRepository(pool),
		PeriodicJobs: NewJetPeriodicJobRepository(pool),
		KV:           NewJetKVRepository(pool),
	}
}

func (r *Repositories) WithTx(ctx context.Context, fn func(ctx context.Context) error) error {
	if _, ok := txFromContext(ctx); ok {
		return fn(ctx)
	}

	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	txCtx := contextWithTx(ctx, tx)
	if err := fn(txCtx); err != nil {
		return err
	}

	return tx.Commit(ctx)
}
