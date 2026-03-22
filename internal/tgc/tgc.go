package tgc

import (
	"context"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/go-faster/errors"
	"github.com/gotd/contrib/clock"
	"github.com/gotd/contrib/middleware/floodwait"
	"github.com/gotd/contrib/middleware/ratelimit"
	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/dcs"
	"github.com/tgdrive/teldrive/internal/cache"
	"github.com/tgdrive/teldrive/internal/config"
	"github.com/tgdrive/teldrive/internal/logging"
	"github.com/tgdrive/teldrive/internal/recovery"
	"github.com/tgdrive/teldrive/internal/retry"
	"github.com/tgdrive/teldrive/internal/tgstorage"
	"github.com/tgdrive/teldrive/internal/utils"
	"github.com/tgdrive/teldrive/pkg/repositories"
	"go.uber.org/zap"
	"golang.org/x/net/proxy"
	"golang.org/x/time/rate"
)

func newClient(ctx context.Context, config *config.TGConfig, handler telegram.UpdateHandler, storage session.Storage, middlewares ...telegram.Middleware) (*telegram.Client, error) {
	resolver, err := resolverFromConfig(config)
	if err != nil {
		return nil, err
	}

	var logger *zap.Logger
	if config.EnableLogging {
		logger = logging.Component("TG")
	}

	opts := telegram.Options{
		Resolver: resolver,
		ReconnectionBackoff: func() backoff.BackOff {
			return newBackoff(config.ReconnectTimeout)
		},
		Device: telegram.DeviceConfig{
			DeviceModel:    config.DeviceModel,
			SystemVersion:  config.SystemVersion,
			AppVersion:     config.AppVersion,
			SystemLangCode: config.SystemLangCode,
			LangPack:       config.LangPack,
			LangCode:       config.LangCode,
		},
		SessionStorage: storage,
		RetryInterval:  2 * time.Second,
		MaxRetries:     20,
		DialTimeout:    10 * time.Second,
		Middlewares:    middlewares,
		UpdateHandler:  handler,
		Logger:         logger,
	}
	if config.Ntp {
		c, err := clock.NewNTP()
		if err != nil {
			return nil, errors.Wrap(err, "create clock")
		}
		opts.Clock = c

	}

	return telegram.NewClient(config.AppID, config.AppHash, opts), nil
}

func resolverFromConfig(config *config.TGConfig) (dcs.Resolver, error) {
	if config == nil {
		return nil, fmt.Errorf("telegram config is nil")
	}

	mtProxyAddr := strings.TrimSpace(config.MTProxy.Addr)
	mtProxySecret := strings.TrimSpace(config.MTProxy.Secret)
	hasMTProxy := mtProxyAddr != "" || mtProxySecret != ""

	if hasMTProxy {
		if strings.TrimSpace(config.Proxy) != "" {
			return nil, fmt.Errorf("tg.proxy and tg.mtproxy cannot be used together")
		}
		if mtProxyAddr == "" {
			return nil, fmt.Errorf("tg.mtproxy.addr is required when tg.mtproxy is configured")
		}
		if mtProxySecret == "" {
			return nil, fmt.Errorf("tg.mtproxy.secret is required when tg.mtproxy is configured")
		}

		secret, err := hex.DecodeString(mtProxySecret)
		if err != nil {
			return nil, errors.Wrap(err, "decode tg.mtproxy.secret")
		}

		resolver, err := dcs.MTProxy(mtProxyAddr, secret, dcs.MTProxyOptions{})
		if err != nil {
			return nil, errors.Wrap(err, "create tg.mtproxy resolver")
		}

		return resolver, nil
	}

	var dialer dcs.DialFunc = proxy.Direct.DialContext
	if config.Proxy != "" {
		d, err := utils.Proxy.GetDial(config.Proxy)
		if err != nil {
			return nil, errors.Wrap(err, "get dialer")
		}
		dialer = d.DialContext
	}

	return dcs.Plain(dcs.PlainOptions{Dial: dialer}), nil
}

func NoAuthClient(ctx context.Context, config *config.TGConfig, handler telegram.UpdateHandler, storage session.Storage) (*telegram.Client, error) {
	middlewares := []telegram.Middleware{
		floodwait.NewSimpleWaiter(),
	}
	middlewares = append(middlewares, ratelimit.New(rate.Every(time.Millisecond*100), 5))
	return newClient(ctx, config, handler, storage, middlewares...)
}

func AuthClient(ctx context.Context, config *config.TGConfig, sessionStr string, middlewares ...telegram.Middleware) (*telegram.Client, error) {
	data, err := session.TelethonSession(sessionStr)

	if err != nil {
		return nil, err
	}

	var (
		storage = new(session.StorageMemory)
		loader  = session.Loader{Storage: storage}
	)

	if err := loader.Save(ctx, data); err != nil {
		return nil, err
	}
	return newClient(ctx, config, nil, storage, middlewares...)
}

// BotClient creates a Telegram client for bot authentication.
// Uses database-backed session storage for persistent bot sessions.
// Note: storage remains open for client's lifetime - do not close it here
func BotClient(ctx context.Context, kvRepo repositories.KVRepository, cache cache.Cacher, config *config.TGConfig, token string, middlewares ...telegram.Middleware) (*telegram.Client, error) {
	// Use bot token ID (part before colon) as session key
	botID := strings.Split(token, ":")[0]
	storage, err := tgstorage.NewSessionStorage(config.Session, kvRepo, cache, botID)
	if err != nil {
		return nil, err
	}
	// Storage must remain open for the client's entire lifetime
	// It will be garbage collected when the client is no longer referenced
	return newClient(ctx, config, nil, storage, middlewares...)
}

type middlewareOption func(*middlewareConfig)

type middlewareConfig struct {
	config      *config.TGConfig
	middlewares []telegram.Middleware
}

func NewMiddleware(config *config.TGConfig, opts ...middlewareOption) []telegram.Middleware {
	mc := &middlewareConfig{
		config:      config,
		middlewares: []telegram.Middleware{},
	}
	for _, opt := range opts {
		opt(mc)
	}
	return mc.middlewares
}

func WithFloodWait() middlewareOption {
	return func(mc *middlewareConfig) {
		mc.middlewares = append(mc.middlewares, floodwait.NewSimpleWaiter())
	}
}

func WithRecovery(ctx context.Context) middlewareOption {
	return func(mc *middlewareConfig) {
		mc.middlewares = append(mc.middlewares,
			recovery.New(ctx, newBackoff(mc.config.ReconnectTimeout)))
	}
}

func WithRetry(retries int) middlewareOption {
	return func(mc *middlewareConfig) {
		mc.middlewares = append(mc.middlewares, retry.New(retries))
	}
}

func WithRateLimit() middlewareOption {
	return func(mc *middlewareConfig) {
		if mc.config.RateLimit {
			mc.middlewares = append(mc.middlewares,
				ratelimit.New(rate.Every(time.Millisecond*time.Duration(mc.config.Rate)), mc.config.RateBurst))
		}
	}
}

func newBackoff(timeout time.Duration) backoff.BackOff {
	b := backoff.NewExponentialBackOff()
	b.Multiplier = 1.1
	b.MaxElapsedTime = timeout
	b.MaxInterval = 10 * time.Second
	return b
}
