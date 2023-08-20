package utils

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/gotd/contrib/bg"
	"github.com/gotd/contrib/middleware/floodwait"
	"github.com/gotd/contrib/middleware/ratelimit"
	tdclock "github.com/gotd/td/clock"
	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"golang.org/x/time/rate"
)

type Client struct {
	Tg       *telegram.Client
	Token    string
	Workload int
}

var clients map[int]*Client

func getDeviceConfig() telegram.DeviceConfig {
	appConfig := GetConfig()
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

func reconnectionBackoff() backoff.BackOff {
	_clock := tdclock.System
	b := backoff.NewExponentialBackOff()
	b.Multiplier = 1.1
	b.MaxElapsedTime = time.Duration(120) * time.Second
	b.Clock = _clock
	return b
}

func getBotClient(appID int, appHash, clientName, sessionDir string) *telegram.Client {

	sessionStorage := &telegram.FileSessionStorage{
		Path: filepath.Join(sessionDir, clientName+".json"),
	}
	middlewares := []telegram.Middleware{floodwait.NewSimpleWaiter()}

	options := telegram.Options{
		SessionStorage:      sessionStorage,
		Middlewares:         middlewares,
		ReconnectionBackoff: reconnectionBackoff,
		RetryInterval:       5 * time.Second,
		MaxRetries:          5,
		Device:              getDeviceConfig(),
		Clock:               tdclock.System,
	}

	client := telegram.NewClient(appID, appHash, options)

	return client

}

func startClient(ctx context.Context, client *Client) (bg.StopFunc, error) {

	stop, err := bg.Connect(client.Tg)

	if err != nil {
		return nil, errors.Wrap(err, "failed to start client")
	}

	tguser, err := client.Tg.Self(ctx)

	if err != nil {

		if _, err := client.Tg.Auth().Bot(ctx, client.Token); err != nil {
			return nil, err
		}
		tguser, _ = client.Tg.Self(ctx)
	}

	Logger.Info("started Client", zap.String("user", tguser.Username))
	return stop, nil
}

func StartBotTgClients() {

	clients = make(map[int]*Client)

	if config.MultiClient {
		sessionDir := "sessions"

		if err := os.MkdirAll(sessionDir, 0700); err != nil {
			return
		}

		var keysToSort []string

		for _, e := range os.Environ() {
			if strings.HasPrefix(e, "MULTI_TOKEN") {
				if i := strings.Index(e, "="); i >= 0 {
					keysToSort = append(keysToSort, e[:i])
				}
			}
		}

		sort.Strings(keysToSort)

		for idx, key := range keysToSort {
			client := getBotClient(config.AppId, config.AppHash, fmt.Sprintf("client%d", idx), sessionDir)
			clients[idx] = &Client{Tg: client, Token: os.Getenv(key)}
		}

		ctx := context.Background()

		for _, client := range clients {
			go startClient(ctx, client)
		}
	}

}

func GetAuthClient(sessionStr string, userId int) (*Client, bg.StopFunc, error) {

	if client, ok := clients[userId]; ok {
		return client, nil, nil
	}

	ctx := context.Background()

	data, err := session.TelethonSession(sessionStr)

	if err != nil {
		return nil, nil, err
	}

	var (
		storage = new(session.StorageMemory)
		loader  = session.Loader{Storage: storage}
	)

	if err := loader.Save(ctx, data); err != nil {
		return nil, nil, err
	}
	middlewares := []telegram.Middleware{floodwait.NewSimpleWaiter()}
	client := telegram.NewClient(config.AppId, config.AppHash, telegram.Options{
		SessionStorage:      storage,
		Middlewares:         middlewares,
		ReconnectionBackoff: reconnectionBackoff,
		RetryInterval:       5 * time.Second,
		MaxRetries:          5,
		Device:              getDeviceConfig(),
		Clock:               tdclock.System,
	})

	stop, err := bg.Connect(client)

	if err != nil {
		return nil, nil, err
	}

	tguser, err := client.Self(ctx)

	if err != nil {
		return nil, nil, err
	}

	Logger.Info("started Client", zap.String("user", tguser.Username))

	tgClient := &Client{Tg: client}

	clients[int(tguser.GetID())] = tgClient

	return tgClient, stop, nil
}

func GetBotClient() *Client {
	smallest := clients[0]
	for _, client := range clients {
		if client.Workload < smallest.Workload {
			smallest = client
		}
	}
	return smallest
}

func GetNonAuthClient(handler telegram.UpdateHandler, storage telegram.SessionStorage) (*telegram.Client, bg.StopFunc, error) {
	middlewares := []telegram.Middleware{}
	if config.RateLimit {
		middlewares = append(middlewares, ratelimit.New(rate.Every(time.Millisecond*100), 5))
	}
	client := telegram.NewClient(config.AppId, config.AppHash, telegram.Options{
		SessionStorage: storage,
		Middlewares:    middlewares,
		Device:         getDeviceConfig(),
		UpdateHandler:  handler,
	})

	stop, err := bg.Connect(client)

	if err != nil {
		return nil, nil, err
	}

	return client, stop, nil
}

func StopClient(stop bg.StopFunc, key int) {
	if stop != nil {
		stop()
	}
	delete(clients, key)
}
