package tgc

import (
	"context"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/divyam234/teldrive/internal/config"
	"github.com/divyam234/teldrive/internal/kv"
	"github.com/divyam234/teldrive/internal/recovery"
	"github.com/divyam234/teldrive/internal/retry"
	"github.com/gotd/contrib/middleware/floodwait"
	"github.com/gotd/contrib/middleware/ratelimit"
	tdclock "github.com/gotd/td/clock"
	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
	"golang.org/x/time/rate"
)

func defaultMiddlewares(ctx context.Context) ([]telegram.Middleware, error) {

	return []telegram.Middleware{
		recovery.New(ctx, Backoff(tdclock.System)),
		retry.New(5),
		floodwait.NewSimpleWaiter(),
	}, nil
}

func New(ctx context.Context, config *config.TelegramConfig, handler telegram.UpdateHandler, storage session.Storage, middlewares ...telegram.Middleware) *telegram.Client {

	_clock := tdclock.System

	noUpdates := true

	if handler != nil {
		noUpdates = false
	}

	opts := telegram.Options{
		ReconnectionBackoff: func() backoff.BackOff {
			return Backoff(_clock)
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
		RetryInterval:  time.Second,
		MaxRetries:     10,
		DialTimeout:    10 * time.Second,
		Middlewares:    middlewares,
		Clock:          _clock,
		NoUpdates:      noUpdates,
		UpdateHandler:  handler,
	}

	return telegram.NewClient(config.AppId, config.AppHash, opts)
}

func NoAuthClient(ctx context.Context, config *config.TelegramConfig, handler telegram.UpdateHandler, storage session.Storage) *telegram.Client {
	middlewares, _ := defaultMiddlewares(ctx)
	middlewares = append(middlewares, ratelimit.New(rate.Every(time.Millisecond*100), 5))
	return New(ctx, config, handler, storage, middlewares...)
}

func AuthClient(ctx context.Context, config *config.TelegramConfig, sessionStr string) (*telegram.Client, error) {
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
	middlewares, _ := defaultMiddlewares(ctx)
	middlewares = append(middlewares, ratelimit.New(rate.Every(time.Millisecond*
		time.Duration(config.Rate)), config.RateBurst))
	return New(ctx, config, nil, storage, middlewares...), nil
}

func BotClient(ctx context.Context, KV kv.KV, config *config.TelegramConfig, token string) (*telegram.Client, error) {
	storage := kv.NewSession(KV, kv.Key("botsession", token))
	middlewares, _ := defaultMiddlewares(ctx)
	if config.RateLimit {
		middlewares = append(middlewares, ratelimit.New(rate.Every(time.Millisecond*
			time.Duration(config.Rate)), config.RateBurst))

	}
	return New(ctx, config, nil, storage, middlewares...), nil
}
func Backoff(_clock tdclock.Clock) backoff.BackOff {
	b := backoff.NewExponentialBackOff()

	b.Multiplier = 1.1
	b.MaxElapsedTime = time.Duration(120) * time.Second
	b.Clock = _clock
	return b
}
