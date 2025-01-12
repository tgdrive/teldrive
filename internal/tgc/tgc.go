package tgc

import (
	"context"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/go-faster/errors"
	tgbbolt "github.com/gotd/contrib/bbolt"
	"github.com/gotd/contrib/clock"
	"github.com/gotd/contrib/middleware/floodwait"
	"github.com/gotd/contrib/middleware/ratelimit"
	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/dcs"
	"github.com/tgdrive/teldrive/internal/config"
	"github.com/tgdrive/teldrive/internal/logging"
	"github.com/tgdrive/teldrive/internal/recovery"
	"github.com/tgdrive/teldrive/internal/retry"
	"github.com/tgdrive/teldrive/internal/utils"
	"go.etcd.io/bbolt"
	"go.uber.org/zap"
	"golang.org/x/net/proxy"
	"golang.org/x/time/rate"
)

func sessionKey(indexes ...string) string {
	return strings.Join(indexes, ":")
}

func newClient(ctx context.Context, config *config.TGConfig, handler telegram.UpdateHandler, storage session.Storage, middlewares ...telegram.Middleware) (*telegram.Client, error) {

	var dialer dcs.DialFunc = proxy.Direct.DialContext
	if config.Proxy != "" {
		d, err := utils.Proxy.GetDial(config.Proxy)
		if err != nil {
			return nil, errors.Wrap(err, "get dialer")
		}
		dialer = d.DialContext
	}

	var logger *zap.Logger
	if config.EnableLogging {
		logger = logging.FromContext(ctx).Named("td")

	}

	opts := telegram.Options{
		Resolver: dcs.Plain(dcs.PlainOptions{
			Dial: dialer,
		}),
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
		MaxRetries:     10,
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

	return telegram.NewClient(config.AppId, config.AppHash, opts), nil
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

	if err := loader.Save(context.TODO(), data); err != nil {
		return nil, err
	}
	return newClient(ctx, config, nil, storage, middlewares...)
}

func BotClient(ctx context.Context, boltdb *bbolt.DB, config *config.TGConfig, token string, middlewares ...telegram.Middleware) (*telegram.Client, error) {

	storage := tgbbolt.NewSessionStorage(boltdb, sessionKey("botsession", token), []byte("teldrive"))

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
