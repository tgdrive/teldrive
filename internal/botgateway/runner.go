package botgateway

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/gotd/td/tg"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tgdrive/teldrive/v2/internal/db/sqlcgen"
	"github.com/tgdrive/teldrive/v2/internal/secureblob"
	"github.com/tgdrive/teldrive/v2/internal/telegramstore"
)

var ErrUploadRunnerConfiguration = errors.New("upload-aware Telegram runner is not configured")

// UploadAwareRunner executes uploads and optionally downloads through an enabled
// bot. Each call goes to the eligible bot with the fewest Telegram connections
// in flight in this process; ties rotate through the selector so idle bots are
// used in turn. Authenticated bot sessions are reused from encrypted Telethon
// StringSession values stored in the bots table.
type UploadAwareRunner struct {
	queries      *sqlcgen.Queries
	selector     botSelector
	load         botLoad
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
	if fn == nil {
		return ErrUploadRunnerConfiguration
	}
	bot, err := r.acquireBot(ctx, userID, operation, connections)
	if err != nil {
		return err
	}
	if bot == nil {
		return r.runFallback(ctx, userID, operation, connections, fn)
	}
	defer r.load.release(bot.BotID, connections)
	if err := r.runOnBot(ctx, userID, bot, connections, fn); err != nil {
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

// LeaseAccount reserves connections of active load on the bot that should serve
// the next unit of work, chosen like Run. Without an eligible bot the lease
// targets the fallback runner (the user's own account) and reserves nothing.
func (r *UploadAwareRunner) LeaseAccount(ctx context.Context, userID int64, operation telegramstore.Operation, connections int) (telegramstore.AccountLease, error) {
	bot, err := r.acquireBot(ctx, userID, operation, connections)
	if err != nil {
		return nil, err
	}
	if bot == nil {
		return &fallbackLease{runner: r, userID: userID, operation: operation}, nil
	}
	return &botLease{runner: r, userID: userID, operation: operation, bot: bot, connections: connections}, nil
}

// acquireBot reserves connections on the least-loaded eligible bot. It returns
// nil when the operation must use the fallback runner.
func (r *UploadAwareRunner) acquireBot(ctx context.Context, userID int64, operation telegramstore.Operation, connections int) (*sqlcgen.Bot, error) {
	if r == nil || r.queries == nil || r.selector == nil || r.cipher == nil || r.factory == nil || r.fallback == nil || userID <= 0 || connections < 1 {
		return nil, ErrUploadRunnerConfiguration
	}
	var bots []*sqlcgen.Bot
	var err error
	switch operation {
	case telegramstore.OperationUpload:
		bots, err = r.queries.ListUploadEligibleBots(ctx, userID)
	case telegramstore.OperationDownload:
		if r.downloadBots == 0 {
			return nil, nil
		}
		bots, err = r.queries.ListEnabledBots(ctx, userID)
		if len(bots) > r.downloadBots {
			bots = bots[:r.downloadBots]
		}
	default:
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("list %s bots: %w", operation, err)
	}
	if len(bots) == 0 {
		return nil, nil
	}
	selection, err := r.selector.Next(ctx, userID, operation)
	if err != nil {
		return nil, fmt.Errorf("select %s bot: %w", operation, err)
	}
	return r.load.acquire(bots, selection, connections), nil
}

func (r *UploadAwareRunner) runOnBot(ctx context.Context, userID int64, bot *sqlcgen.Bot, connections int, fn func(context.Context, *tg.Client) error) error {
	if r.runBotFunc != nil {
		return r.runBotFunc(ctx, userID, bot, connections, fn)
	}
	return r.runBot(ctx, userID, bot, connections, fn)
}

func (r *UploadAwareRunner) runBot(ctx context.Context, userID int64, bot *sqlcgen.Bot, connections int, fn func(context.Context, *tg.Client) error) error {
	storage := &botSessionStorage{
		queries: r.queries, cipher: r.cipher, userID: userID, botID: bot.BotID,
		stored: append([]byte(nil), bot.Session...),
	}
	client, err := r.factory.NewForAccount(storage, bot.BotID)
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
			pooled, closePool, err := r.factory.PooledAPI(client, bot.BotID, connections)
			if err != nil {
				return fmt.Errorf("create pooled API: %w", err)
			}
			defer closePool()
			api = pooled
		}
		return fn(telegramstore.WithClientID(runCtx, bot.BotID), api)
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

// botLoad counts the Telegram connections of active work on each bot in this
// process: running calls and leased download sessions. Idle warm clients are
// not counted. Counts are keyed by bot ID, so a bot registered by several users
// is one load.
type botLoad struct {
	mu          sync.Mutex
	connections map[int64]int
}

// acquire reserves connections on the least-loaded bot. The scan starts at the
// rotation offset, so equally loaded bots are chosen in round-robin order.
func (l *botLoad) acquire(bots []*sqlcgen.Bot, rotation uint64, connections int) *sqlcgen.Bot {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.connections == nil {
		l.connections = make(map[int64]int)
	}
	start := int(rotation % uint64(len(bots)))
	best := bots[start]
	for offset := 1; offset < len(bots); offset++ {
		if bot := bots[(start+offset)%len(bots)]; l.connections[bot.BotID] < l.connections[best.BotID] {
			best = bot
		}
	}
	l.connections[best.BotID] += connections
	return best
}

func (l *botLoad) release(botID int64, connections int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if remaining := l.connections[botID] - connections; remaining > 0 {
		l.connections[botID] = remaining
	} else {
		delete(l.connections, botID)
	}
}

type botLease struct {
	runner      *UploadAwareRunner
	userID      int64
	operation   telegramstore.Operation
	bot         *sqlcgen.Bot
	connections int
	release     sync.Once
}

func (l *botLease) AccountID() int64 { return l.bot.BotID }

func (l *botLease) RunPooled(ctx context.Context, connections int, fn func(context.Context, *tg.Client) error) error {
	if err := l.runner.runOnBot(ctx, l.userID, l.bot, connections, fn); err != nil {
		return fmt.Errorf("%s with bot %d: %w", l.operation, l.bot.BotID, err)
	}
	return nil
}

func (l *botLease) Release() {
	l.release.Do(func() { l.runner.load.release(l.bot.BotID, l.connections) })
}

type fallbackLease struct {
	runner    *UploadAwareRunner
	userID    int64
	operation telegramstore.Operation
}

func (l *fallbackLease) AccountID() int64 { return l.userID }

func (l *fallbackLease) RunPooled(ctx context.Context, connections int, fn func(context.Context, *tg.Client) error) error {
	return l.runner.runFallback(ctx, l.userID, l.operation, connections, fn)
}

func (l *fallbackLease) Release() {}

var _ telegramstore.Runner = (*UploadAwareRunner)(nil)
var _ telegramstore.PooledRunner = (*UploadAwareRunner)(nil)
var _ telegramstore.AccountLeaser = (*UploadAwareRunner)(nil)
