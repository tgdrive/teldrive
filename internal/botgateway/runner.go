package botgateway

import (
	"context"
	"errors"
	"fmt"

	"github.com/gotd/td/tg"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tgdrive/teldrive/v2/internal/db/sqlcgen"
	"github.com/tgdrive/teldrive/v2/internal/secureblob"
	"github.com/tgdrive/teldrive/v2/internal/telegramstore"
)

var ErrUploadRunnerConfiguration = errors.New("upload-aware Telegram runner is not configured")

// UploadAwareRunner executes uploads and optionally downloads through an enabled
// bot. Selection is independent per operation, and authenticated bot sessions
// are reused from encrypted Telethon StringSession values stored in the bots table.
type UploadAwareRunner struct {
	queries      *sqlcgen.Queries
	selector     botSelector
	cipher       *secureblob.Cipher
	factory      *telegramstore.Factory
	fallback     telegramstore.Runner
	downloadBots int
	runBotFunc   func(context.Context, int64, *sqlcgen.Bot, int, func(context.Context, *tg.Client) error) error
}

func NewUploadAwareRunner(pool *pgxpool.Pool, cipher *secureblob.Cipher, factory *telegramstore.Factory, fallback telegramstore.Runner, downloadBots int, rotation ...string) (*UploadAwareRunner, error) {
	if pool == nil || cipher == nil || factory == nil || fallback == nil || downloadBots < 0 {
		return nil, ErrUploadRunnerConfiguration
	}
	queries := sqlcgen.New(pool)
	mode := RotationMemory
	if len(rotation) > 0 && rotation[0] != "" {
		mode = rotation[0]
	}
	var selector botSelector
	switch mode {
	case RotationMemory:
		selector = &memoryBotSelector{}
	case RotationDatabase:
		selector = databaseBotSelector{queries: queries}
	default:
		return nil, fmt.Errorf("%w: unsupported bot rotation backend %q", ErrUploadRunnerConfiguration, mode)
	}
	return &UploadAwareRunner{
		queries: queries, selector: selector, cipher: cipher, factory: factory, fallback: fallback,
		downloadBots: downloadBots,
	}, nil
}

func (r *UploadAwareRunner) Run(ctx context.Context, userID int64, operation telegramstore.Operation, fn func(context.Context, *tg.Client) error) error {
	return r.run(ctx, userID, operation, 1, fn)
}

func (r *UploadAwareRunner) RunPooled(ctx context.Context, userID int64, operation telegramstore.Operation, connections int, fn func(context.Context, *tg.Client) error) error {
	return r.run(ctx, userID, operation, connections, fn)
}

func (r *UploadAwareRunner) run(ctx context.Context, userID int64, operation telegramstore.Operation, connections int, fn func(context.Context, *tg.Client) error) error {
	if r == nil || r.queries == nil || r.selector == nil || r.cipher == nil || r.factory == nil || r.fallback == nil || userID <= 0 || connections < 1 || fn == nil {
		return ErrUploadRunnerConfiguration
	}
	var bots []*sqlcgen.Bot
	var err error
	switch operation {
	case telegramstore.OperationUpload:
		bots, err = r.queries.ListUploadEligibleBots(ctx, userID)
	case telegramstore.OperationDownload:
		if r.downloadBots == 0 {
			return r.runFallback(ctx, userID, operation, connections, fn)
		}
		bots, err = r.queries.ListEnabledBots(ctx, userID)
		if len(bots) > r.downloadBots {
			bots = bots[:r.downloadBots]
		}
	default:
		return r.runFallback(ctx, userID, operation, connections, fn)
	}
	if err != nil {
		return fmt.Errorf("list %s bots: %w", operation, err)
	}
	if len(bots) == 0 {
		return r.runFallback(ctx, userID, operation, connections, fn)
	}
	selection, err := r.selector.Next(ctx, userID, operation)
	if err != nil {
		return fmt.Errorf("select %s bot: %w", operation, err)
	}
	bot := bots[int(selection%uint64(len(bots)))]
	runBot := r.runBot
	if r.runBotFunc != nil {
		runBot = r.runBotFunc
	}
	if err := runBot(ctx, userID, bot, connections, fn); err != nil {
		if operation == telegramstore.OperationUpload {
			_, _ = r.queries.MarkBotUploadFailure(ctx, sqlcgen.MarkBotUploadFailureParams{
				UserID: userID, BotID: bot.BotID, LastError: err.Error(),
			})
		}
		return fmt.Errorf("%s with bot %d: %w", operation, bot.BotID, err)
	}
	if operation == telegramstore.OperationUpload {
		_, _ = r.queries.MarkBotUploadSuccess(ctx, sqlcgen.MarkBotUploadSuccessParams{UserID: userID, BotID: bot.BotID})
	}
	return nil
}

func (r *UploadAwareRunner) runBot(ctx context.Context, userID int64, bot *sqlcgen.Bot, connections int, fn func(context.Context, *tg.Client) error) error {
	storage := &botSessionStorage{
		queries: r.queries, cipher: r.cipher, userID: userID, botID: bot.BotID,
		stored: append([]byte(nil), bot.Session...),
	}
	client, err := r.factory.New(storage)
	if err != nil {
		return fmt.Errorf("create client: %w", err)
	}
	err = client.Run(ctx, func(runCtx context.Context) error {
		status, err := client.Auth().Status(runCtx)
		if err != nil {
			return fmt.Errorf("check authorization: %w", err)
		}
		if !status.Authorized {
			token, err := r.cipher.Open("bot-token", bot.TokenCiphertext)
			if err != nil {
				return fmt.Errorf("decrypt token: %w", err)
			}
			if _, err := client.Auth().Bot(runCtx, string(token)); err != nil {
				return fmt.Errorf("authenticate: %w", err)
			}
		}
		api := client.API()
		if connections > 1 {
			pooled, closePool, err := r.factory.PooledAPI(client, connections)
			if err != nil {
				return fmt.Errorf("create pooled API: %w", err)
			}
			defer closePool()
			api = pooled
		}
		return fn(runCtx, api)
	})
	if err != nil {
		return err
	}
	return nil
}

func (r *UploadAwareRunner) runFallback(ctx context.Context, userID int64, operation telegramstore.Operation, connections int, fn func(context.Context, *tg.Client) error) error {
	if pooled, ok := r.fallback.(telegramstore.PooledRunner); ok && connections > 1 {
		return pooled.RunPooled(ctx, userID, operation, connections, fn)
	}
	return r.fallback.Run(ctx, userID, operation, fn)
}

var _ telegramstore.Runner = (*UploadAwareRunner)(nil)
var _ telegramstore.PooledRunner = (*UploadAwareRunner)(nil)
