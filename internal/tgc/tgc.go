package tgc

import (
	"context"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/divyam234/teldrive/config"
	"github.com/divyam234/teldrive/internal/kv"
	"github.com/divyam234/teldrive/internal/recovery"
	"github.com/divyam234/teldrive/internal/retry"
	"github.com/divyam234/teldrive/pkg/database"
	"github.com/gotd/contrib/middleware/floodwait"
	"github.com/gotd/contrib/middleware/ratelimit"
	tdclock "github.com/gotd/td/clock"
	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
	"golang.org/x/time/rate"
)

func deviceConfig() telegram.DeviceConfig {
	appConfig := config.GetConfig()
	config := telegram.DeviceConfig{
		DeviceModel:    appConfig.TgClientDeviceModel,
		SystemVersion:  appConfig.TgClientSystemVersion,
		AppVersion:     appConfig.TgClientAppVersion,
		SystemLangCode: appConfig.TgClientSystemLangCode,
		LangPack:       appConfig.TgClientLangPack,
		LangCode:       appConfig.TgClientLangCode,
	}
	return config
}
func NewDefaultMiddlewares(ctx context.Context) ([]telegram.Middleware, error) {

	return []telegram.Middleware{
		recovery.New(ctx, Backoff(tdclock.System)),
		retry.New(5),
		floodwait.NewSimpleWaiter(),
	}, nil
}

func New(ctx context.Context, handler telegram.UpdateHandler, storage session.Storage, middlewares ...telegram.Middleware) *telegram.Client {

	_clock := tdclock.System

	noUpdates := true

	if handler != nil {
		noUpdates = false
	}

	opts := telegram.Options{
		ReconnectionBackoff: func() backoff.BackOff {
			return Backoff(_clock)
		},
		Device:         deviceConfig(),
		SessionStorage: storage,
		RetryInterval:  time.Second,
		MaxRetries:     10,
		DialTimeout:    10 * time.Second,
		Middlewares:    middlewares,
		Clock:          _clock,
		NoUpdates:      noUpdates,
		UpdateHandler:  handler,
	}

	return telegram.NewClient(config.GetConfig().AppId, config.GetConfig().AppHash, opts)
}

func NoLogin(ctx context.Context, handler telegram.UpdateHandler, storage session.Storage) *telegram.Client {
	middlewares, _ := NewDefaultMiddlewares(ctx)
	middlewares = append(middlewares, ratelimit.New(rate.Every(time.Millisecond*100), 5))
	return New(ctx, handler, storage, middlewares...)
}

func UserLogin(ctx context.Context, sessionStr string) (*telegram.Client, error) {
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
	middlewares, _ := NewDefaultMiddlewares(ctx)
	middlewares = append(middlewares, ratelimit.New(rate.Every(time.Millisecond*time.Duration(config.GetConfig().Rate)), config.GetConfig().RateBurst))
	return New(ctx, nil, storage, middlewares...), nil
}

func BotLogin(ctx context.Context, token string) (*telegram.Client, error) {
	storage := kv.NewSession(database.KV, kv.Key("botsession", token))
	middlewares, _ := NewDefaultMiddlewares(ctx)
	if config.GetConfig().RateLimit {
		middlewares = append(middlewares, ratelimit.New(rate.Every(time.Millisecond*time.Duration(config.GetConfig().Rate)), config.GetConfig().RateBurst))

	}
	return New(ctx, nil, storage, middlewares...), nil
}
func Backoff(_clock tdclock.Clock) backoff.BackOff {
	b := backoff.NewExponentialBackOff()

	b.Multiplier = 1.1
	b.MaxElapsedTime = time.Duration(120) * time.Second
	b.Clock = _clock
	return b
}
