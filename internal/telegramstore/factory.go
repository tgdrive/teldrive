package telegramstore

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/google/uuid"
	"github.com/gotd/contrib/middleware/floodwait"
	"github.com/gotd/contrib/middleware/ratelimit"
	"github.com/gotd/log"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/dcs"
	"github.com/gotd/td/tg"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/net/proxy"
	"golang.org/x/time/rate"

	"github.com/tgdrive/teldrive/v2/internal/db/sqlcgen"
	"github.com/tgdrive/teldrive/v2/internal/dbtypes"
	"github.com/tgdrive/teldrive/v2/internal/principal"
	"github.com/tgdrive/teldrive/v2/internal/secureblob"
	"github.com/tgdrive/teldrive/v2/internal/telethonsession"
)

var ErrTelegramConfiguration = errors.New("Telegram client factory is not configured")

type FactoryConfig struct {
	AppID            int
	AppHash          string
	Device           telegram.DeviceConfig
	DialTimeout      time.Duration
	ReconnectTimeout time.Duration
	MaxRetries       int
	RateLimit        bool
	RateInterval     time.Duration
	RateBurst        int
	Proxy            string
	MTProxyAddress   string
	MTProxySecret    string
	Logger           log.Logger
}

// Factory creates unstarted gotd clients. Each caller must execute the client
// with Client.Run for exactly one request-scoped operation. Rate limiting is
// applied per Telegram account, so adding bots adds request budget.
type Factory struct {
	config     FactoryConfig
	resolver   dcs.Resolver
	middleware []telegram.Middleware

	limitersMu sync.Mutex
	limiters   map[int64]*ratelimit.RateLimiter
}

func NewFactory(config FactoryConfig) (*Factory, error) {
	if config.AppID <= 0 || strings.TrimSpace(config.AppHash) == "" {
		return nil, ErrTelegramConfiguration
	}
	if config.DialTimeout <= 0 {
		config.DialTimeout = 15 * time.Second
	}
	if config.ReconnectTimeout <= 0 {
		config.ReconnectTimeout = 5 * time.Minute
	}
	if config.MaxRetries < 0 {
		return nil, fmt.Errorf("%w: max retries cannot be negative", ErrTelegramConfiguration)
	}
	if config.Device.SystemVersion == "" {
		config.Device.SystemVersion = "TelDrive Backend v2"
	}
	if config.Device.AppVersion == "" {
		config.Device.AppVersion = "2"
	}
	if config.Device.DeviceModel == "" {
		config.Device.DeviceModel = "Server"
	}
	resolver, err := resolverFromConfig(config)
	if err != nil {
		return nil, err
	}
	retry, err := newRetryMiddleware(config.MaxRetries)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrTelegramConfiguration, err)
	}
	if config.RateLimit && (config.RateInterval <= 0 || config.RateBurst < 1) {
		return nil, fmt.Errorf("%w: rate interval and burst must be positive", ErrTelegramConfiguration)
	}
	return &Factory{
		config: config, resolver: resolver,
		middleware: []telegram.Middleware{floodwait.NewSimpleWaiter(), retry},
		limiters:   make(map[int64]*ratelimit.RateLimiter),
	}, nil
}

func resolverFromConfig(config FactoryConfig) (dcs.Resolver, error) {
	proxyURL := strings.TrimSpace(config.Proxy)
	mtAddress := strings.TrimSpace(config.MTProxyAddress)
	mtSecret := strings.TrimSpace(config.MTProxySecret)
	if (mtAddress == "") != (mtSecret == "") {
		return nil, fmt.Errorf("%w: MTProxy address and secret must be configured together", ErrTelegramConfiguration)
	}
	if proxyURL != "" && mtAddress != "" {
		return nil, fmt.Errorf("%w: proxy and MTProxy cannot be used together", ErrTelegramConfiguration)
	}
	if mtAddress != "" {
		secret, err := hex.DecodeString(mtSecret)
		if err != nil {
			return nil, fmt.Errorf("%w: decode MTProxy secret: %v", ErrTelegramConfiguration, err)
		}
		resolver, err := dcs.MTProxy(mtAddress, secret, dcs.MTProxyOptions{})
		if err != nil {
			return nil, fmt.Errorf("%w: create MTProxy resolver: %v", ErrTelegramConfiguration, err)
		}
		return resolver, nil
	}

	var dialer proxy.ContextDialer = proxy.Direct
	if proxyURL != "" {
		parsed, err := url.Parse(proxyURL)
		if err != nil {
			return nil, fmt.Errorf("%w: parse proxy URL: %v", ErrTelegramConfiguration, err)
		}
		if parsed.Scheme == "http" || parsed.Scheme == "https" {
			dialer, err = newHTTPConnectDialer(parsed, config.DialTimeout)
			if err != nil {
				return nil, fmt.Errorf("%w: create HTTP proxy dialer: %v", ErrTelegramConfiguration, err)
			}
		} else {
			created, err := proxy.FromURL(parsed, proxy.Direct)
			if err != nil {
				return nil, fmt.Errorf("%w: create proxy dialer: %v", ErrTelegramConfiguration, err)
			}
			contextDialer, ok := created.(proxy.ContextDialer)
			if !ok {
				return nil, fmt.Errorf("%w: proxy dialer does not support context cancellation", ErrTelegramConfiguration)
			}
			dialer = contextDialer
		}
	}
	return dcs.Plain(dcs.PlainOptions{Dial: dialer.DialContext}), nil
}

// New creates a client that is not bound to a known Telegram account, such as
// a login or token verification client. It gets its own rate-limit budget.
func (f *Factory) New(storage telegram.SessionStorage) (*telegram.Client, error) {
	return f.newClient(storage, 0, nil)
}

// NewForAccount creates a client for an existing Telegram user or bot account.
// All clients and connection pools for the same account share one rate-limit
// budget, because Telegram enforces flood limits per account.
func (f *Factory) NewForAccount(storage telegram.SessionStorage, accountID int64) (*telegram.Client, error) {
	if accountID <= 0 {
		return nil, ErrTelegramConfiguration
	}
	return f.newClient(storage, accountID, nil)
}

func (f *Factory) NewWithUpdates(storage telegram.SessionStorage, handler telegram.UpdateHandler) (*telegram.Client, error) {
	if handler == nil {
		return nil, ErrTelegramConfiguration
	}
	return f.newClient(storage, 0, handler)
}

// PooledAPI opens size connections on client and charges their requests to the
// rate-limit budget of accountID.
func (f *Factory) PooledAPI(client *telegram.Client, accountID int64, size int) (*tg.Client, func() error, error) {
	if f == nil || client == nil || accountID <= 0 || size < 1 {
		return nil, nil, ErrTelegramConfiguration
	}
	invoker, err := client.Pool(int64(size))
	if err != nil {
		return nil, nil, fmt.Errorf("create Telegram connection pool: %w", err)
	}
	return tg.NewClient(f.wrapInvoker(accountID, invoker)), invoker.Close, nil
}

// accountMiddleware returns a fresh invoke middleware chain for one Telegram
// account. A positive accountID shares its limiter with every other client of
// that account; zero gets a limiter of its own.
func (f *Factory) accountMiddleware(accountID int64) []telegram.Middleware {
	if !f.config.RateLimit {
		return slices.Clone(f.middleware)
	}
	if accountID <= 0 {
		return append(slices.Clip(f.middleware), f.newLimiter())
	}
	f.limitersMu.Lock()
	limiter, ok := f.limiters[accountID]
	if !ok {
		limiter = f.newLimiter()
		f.limiters[accountID] = limiter
	}
	f.limitersMu.Unlock()
	return append(slices.Clip(f.middleware), limiter)
}

func (f *Factory) newLimiter() *ratelimit.RateLimiter {
	return ratelimit.New(rate.Every(f.config.RateInterval), f.config.RateBurst)
}

func (f *Factory) wrapInvoker(accountID int64, invoker tg.Invoker) tg.Invoker {
	middleware := f.accountMiddleware(accountID)
	for i := len(middleware) - 1; i >= 0; i-- {
		invoker = middleware[i].Handle(invoker)
	}
	return invoker
}

func (f *Factory) AppCredentials() (int, string, bool) {
	if f == nil || f.config.AppID <= 0 || strings.TrimSpace(f.config.AppHash) == "" {
		return 0, "", false
	}
	return f.config.AppID, f.config.AppHash, true
}
func (f *Factory) newClient(storage telegram.SessionStorage, accountID int64, handler telegram.UpdateHandler) (*telegram.Client, error) {
	if f == nil || f.config.AppID <= 0 || f.config.AppHash == "" || storage == nil {
		return nil, ErrTelegramConfiguration
	}
	options := telegram.Options{
		SessionStorage: storage,
		UpdateHandler:  handler,
		NoUpdates:      handler == nil,
		DialTimeout:    f.config.DialTimeout,
		MaxRetries:     f.config.MaxRetries,
		Device:         f.config.Device,
		Resolver:       f.resolver,
		Middlewares:    f.accountMiddleware(accountID),
		Logger:         f.config.Logger,
		RetryInterval:  2 * time.Second,
		ReconnectionBackoff: func() backoff.BackOff {
			value := backoff.NewExponentialBackOff()
			value.Multiplier = 1.1
			value.MaxElapsedTime = f.config.ReconnectTimeout
			value.MaxInterval = 10 * time.Second
			return value
		},
	}
	return telegram.NewClient(f.config.AppID, f.config.AppHash, options), nil
}

// DatabaseClientProvider loads one encrypted Telegram session for the user,
// constructs a fresh gotd client, and returns it unstarted. ClientRunner owns
// Client.Run and closes all network state when the request ends.
type DatabaseClientProvider struct {
	queries *sqlcgen.Queries
	cipher  *secureblob.Cipher
	factory *Factory
}

func NewDatabaseClientProvider(pool *pgxpool.Pool, cipher *secureblob.Cipher, factory *Factory) (*DatabaseClientProvider, error) {
	if pool == nil || cipher == nil || factory == nil {
		return nil, ErrTelegramConfiguration
	}
	return &DatabaseClientProvider{queries: sqlcgen.New(pool), cipher: cipher, factory: factory}, nil
}

func (p *DatabaseClientProvider) Client(ctx context.Context, userID int64, _ Operation) (*telegram.Client, error) {
	if p == nil || p.queries == nil || p.cipher == nil || p.factory == nil || userID <= 0 {
		return nil, ErrTelegramConfiguration
	}
	var stored *sqlcgen.Session
	var err error
	identity, hasIdentity := principal.FromContext(ctx)
	if hasIdentity && identity.UserID == userID && identity.SessionID != uuid.Nil {
		stored, err = p.queries.GetActiveSession(ctx, sqlcgen.GetActiveSessionParams{
			SessionID: dbtypes.UUID(identity.SessionID), UserID: userID,
		})
	} else {
		stored, err = p.queries.GetLatestActiveSessionForUser(ctx, userID)
	}
	if err != nil {
		return nil, fmt.Errorf("load active Telegram session: %w", err)
	}
	storage := &databaseSessionStorage{
		queries: p.queries, cipher: p.cipher, sessionID: stored.ID, userID: userID,
	}
	return p.factory.NewForAccount(storage, userID)
}

type databaseSessionStorage struct {
	queries   *sqlcgen.Queries
	cipher    *secureblob.Cipher
	sessionID pgtype.UUID
	userID    int64
}

func (s *databaseSessionStorage) LoadSession(ctx context.Context) ([]byte, error) {
	if s == nil || s.queries == nil || s.cipher == nil || s.userID <= 0 || !s.sessionID.Valid {
		return nil, ErrTelegramConfiguration
	}
	row, err := s.queries.GetActiveSession(ctx, sqlcgen.GetActiveSessionParams{
		SessionID: s.sessionID, UserID: s.userID,
	})
	if err != nil {
		return nil, fmt.Errorf("load encrypted Telegram session: %w", err)
	}
	plain, err := s.cipher.Open("telegram-session", row.TelegramSession)
	if err != nil {
		return nil, fmt.Errorf("decrypt Telegram session: %w", err)
	}
	raw, err := telethonsession.DecodeToGotd(ctx, string(plain))
	if err != nil {
		return nil, fmt.Errorf("load Telethon Telegram session: %w", err)
	}
	return raw, nil
}

func (s *databaseSessionStorage) StoreSession(ctx context.Context, data []byte) error {
	if s == nil || s.queries == nil || s.cipher == nil || len(data) == 0 || !s.sessionID.Valid {
		return ErrTelegramConfiguration
	}
	encoded, err := telethonsession.EncodeGotd(ctx, data)
	if err != nil {
		return fmt.Errorf("encode Telegram session update as Telethon: %w", err)
	}
	ciphertext, err := s.cipher.Seal("telegram-session", []byte(encoded))
	if err != nil {
		return fmt.Errorf("encrypt Telegram session update: %w", err)
	}
	count, err := s.queries.UpdateSessionTelegramSession(ctx, sqlcgen.UpdateSessionTelegramSessionParams{
		TelegramSession: ciphertext, SessionID: s.sessionID,
	})
	if err != nil {
		return fmt.Errorf("store Telegram session update: %w", err)
	}
	if count == 0 {
		return ErrClientUnavailable
	}
	return nil
}

var (
	_ ClientProvider          = (*DatabaseClientProvider)(nil)
	_ telegram.SessionStorage = (*databaseSessionStorage)(nil)
)
