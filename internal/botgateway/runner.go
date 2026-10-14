package botgateway

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tgdrive/teldrive/v2/internal/db/sqlcgen"
	"github.com/tgdrive/teldrive/v2/internal/secureblob"
	"github.com/tgdrive/teldrive/v2/internal/telegramstore"
)

var ErrUploadRunnerConfiguration = errors.New("upload-aware Telegram runner is not configured")

// revokedBotTokenErrors are the RPC errors with which Telegram rejects a bot
// token that was revoked or deleted in BotFather. No retry can succeed until
// the bot is re-added with a new token, so the bot is disabled. Stale-session
// errors such as AUTH_KEY_UNREGISTERED are not listed: runBot treats them as an
// unauthorized session and re-imports the token. SESSION_REVOKED,
// USER_DEACTIVATED and similar errors concern user-account logins.
var revokedBotTokenErrors = []string{"ACCESS_TOKEN_EXPIRED", "ACCESS_TOKEN_INVALID"}

// errBotUnreachable reports that a download bot's first connection attempt
// failed before the bot was ready.
var errBotUnreachable = errors.New("could not connect to Telegram")

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
	for {
		bot, err := r.acquireBot(ctx, userID, operation, connections)
		if err != nil {
			return err
		}
		if bot == nil {
			return r.runFallback(ctx, userID, operation, connections, fn)
		}
		started, err := r.runAcquired(ctx, userID, operation, bot, connections, fn)
		// A revoked bot is disabled and no longer eligible. Retrying on the
		// next bot is safe only while fn has not run on the revoked one.
		if started || !errors.Is(err, telegramstore.ErrAccountRevoked) {
			return err
		}
	}
}

func (r *UploadAwareRunner) runAcquired(ctx context.Context, userID int64, operation telegramstore.Operation, bot *sqlcgen.Bot, connections int, fn func(context.Context, *tg.Client) error) (bool, error) {
	defer r.load.release(bot.BotID, connections)
	started, err := r.runOnBotRecorded(ctx, userID, operation, bot, connections, fn)
	if err != nil {
		return started, err
	}
	if operation == telegramstore.OperationUpload {
		_, _ = r.queries.MarkBotUploadSuccess(ctx, sqlcgen.MarkBotUploadSuccessParams{UserID: userID, BotID: bot.BotID})
	}
	return true, nil
}

// runOnBotRecorded runs fn on bot, records any failure against the bot, and
// reports whether fn started.
func (r *UploadAwareRunner) runOnBotRecorded(ctx context.Context, userID int64, operation telegramstore.Operation, bot *sqlcgen.Bot, connections int, fn func(context.Context, *tg.Client) error) (bool, error) {
	started := false
	err := r.runOnBot(ctx, userID, operation, bot, connections, func(runCtx context.Context, api *tg.Client) error {
		started = true
		return fn(runCtx, api)
	})
	if err != nil {
		err = fmt.Errorf("%s with bot %d: %w", operation, bot.BotID, r.recordBotFailure(ctx, userID, operation, bot.BotID, started, err))
	}
	return started, err
}

// recordBotFailure disables a bot whose token Telegram revoked and returns err
// wrapped with telegramstore.ErrAccountRevoked. Other failures that point at
// the bot back it off briefly, since they may succeed on a later attempt:
// every upload failure, and download failures where the bot could not connect
// or authenticate. Downloads that fail once fn runs usually fail on any bot
// (expired file reference, deleted message), and a done ctx means the caller
// gave up, so neither says anything about the bot unless a connection attempt
// had already failed.
func (r *UploadAwareRunner) recordBotFailure(ctx context.Context, userID int64, operation telegramstore.Operation, botID int64, started bool, err error) error {
	if tgerr.Is(err, revokedBotTokenErrors...) {
		if _, disableErr := r.queries.MarkBotTokenRevoked(ctx, sqlcgen.MarkBotTokenRevokedParams{
			UserID: userID, BotID: botID, LastError: err.Error(),
		}); disableErr != nil {
			return errors.Join(err, fmt.Errorf("disable revoked bot: %w", disableErr))
		}
		return fmt.Errorf("%w: %w", telegramstore.ErrAccountRevoked, err)
	}
	if operation == telegramstore.OperationUpload ||
		operation == telegramstore.OperationDownload && !started && (ctx.Err() == nil || errors.Is(err, errBotUnreachable)) {
		_, _ = r.queries.MarkBotFailure(context.WithoutCancel(ctx), sqlcgen.MarkBotFailureParams{
			UserID: userID, BotID: botID, LastError: err.Error(),
		})
	}
	return err
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
		bots, err = r.queries.ListDownloadEligibleBots(ctx, sqlcgen.ListDownloadEligibleBotsParams{
			UserID: userID, DownloadBots: int32(r.downloadBots),
		})
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

func (r *UploadAwareRunner) runOnBot(ctx context.Context, userID int64, operation telegramstore.Operation, bot *sqlcgen.Bot, connections int, fn func(context.Context, *tg.Client) error) error {
	if r.runBotFunc != nil {
		return r.runBotFunc(ctx, userID, bot, connections, fn)
	}
	return r.runBot(ctx, userID, bot, connections, operation == telegramstore.OperationDownload, fn)
}

// runBot runs fn on a client logged in as bot. With failFast, the first failed
// connection attempt before the client is ready ends the call with
// errBotUnreachable. Otherwise gotd keeps reconnecting until ctx is done or the
// reconnect timeout (five minutes by default) passes, so callers that give up
// sooner would never learn that the bot is unreachable.
func (r *UploadAwareRunner) runBot(ctx context.Context, userID int64, bot *sqlcgen.Bot, connections int, failFast bool, fn func(context.Context, *tg.Client) error) error {
	storage := &botSessionStorage{
		queries: r.queries, cipher: r.cipher, userID: userID, botID: bot.BotID,
		stored: append([]byte(nil), bot.Session...),
	}
	clientCtx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	var connected atomic.Bool
	var onConnectionState func(telegram.ConnectionState)
	if failFast {
		onConnectionState = func(state telegram.ConnectionState) {
			if state == telegram.ConnectionStateDisconnected && !connected.Load() {
				cancel(errBotUnreachable)
			}
		}
	}
	client, err := r.factory.NewForAccount(storage, bot.BotID, onConnectionState)
	if err != nil {
		return fmt.Errorf("create client: %w", err)
	}
	err = client.Run(clientCtx, func(runCtx context.Context) error {
		connected.Store(true)
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
	// gotd returns nil when its context is cancelled, so check the cause.
	if errors.Is(context.Cause(clientCtx), errBotUnreachable) {
		return errBotUnreachable
	}
	return err
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
	_, err := l.runner.runOnBotRecorded(ctx, l.userID, l.operation, l.bot, connections, fn)
	return err
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
