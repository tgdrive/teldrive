package utils

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/divyam234/teldrive/types"
	"github.com/gin-gonic/gin"
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

var clients map[int64]*telegram.Client

var Workloads map[int]int

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

func GetBotClient(clientName string) *telegram.Client {

	config := GetConfig()
	sessionStorage := &telegram.FileSessionStorage{
		Path: filepath.Join(config.ExecDir, "sessions", clientName+".json"),
	}

	middlewares := []telegram.Middleware{floodwait.NewSimpleWaiter()}

	if config.RateLimit {
		middlewares = append(middlewares, ratelimit.New(rate.Every(time.Millisecond*100), 5))
	}

	options := telegram.Options{
		SessionStorage:      sessionStorage,
		Middlewares:         middlewares,
		ReconnectionBackoff: reconnectionBackoff,
		RetryInterval:       5 * time.Second,
		MaxRetries:          5,
		Device:              getDeviceConfig(),
		Clock:               tdclock.System,
	}

	client := telegram.NewClient(config.AppId, config.AppHash, options)

	return client

}

func GetAuthClient(ctx context.Context, sessionStr string, userId int64) (*telegram.Client, error) {

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

	middlewares := []telegram.Middleware{floodwait.NewSimpleWaiter()}

	if config.RateLimit {
		middlewares = append(middlewares, ratelimit.New(rate.Every(time.Millisecond*100), 5))
	}

	client := telegram.NewClient(config.AppId, config.AppHash, telegram.Options{
		SessionStorage:      storage,
		Middlewares:         middlewares,
		ReconnectionBackoff: reconnectionBackoff,
		RetryInterval:       5 * time.Second,
		MaxRetries:          5,
		Device:              getDeviceConfig(),
		Clock:               tdclock.System,
	})

	return client, nil
}

func StartNonAuthClient(handler telegram.UpdateHandler, storage telegram.SessionStorage) (*telegram.Client, bg.StopFunc, error) {
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

func startBotClient(ctx context.Context, client *telegram.Client, token string) (bg.StopFunc, error) {

	stop, err := bg.Connect(client)

	if err != nil {
		return nil, errors.Wrap(err, "failed to start client")
	}

	tguser, err := client.Self(ctx)

	if err != nil {

		if _, err := client.Auth().Bot(ctx, token); err != nil {
			return nil, err
		}
		tguser, _ = client.Self(ctx)
	}

	Logger.Info("started Client", zap.String("user", tguser.Username))
	return stop, nil
}

func startAuthClient(c *gin.Context, client *telegram.Client) (bg.StopFunc, error) {
	stop, err := bg.Connect(client)

	if err != nil {
		return nil, err
	}

	tguser, err := client.Self(c)

	if err != nil {
		return nil, err
	}

	Logger.Info("started Client", zap.String("user", tguser.Username))

	clients[tguser.GetID()] = client

	return stop, nil
}

func InitBotClients() {

	ctx := context.Background()

	clients = make(map[int64]*telegram.Client)
	Workloads = make(map[int]int)

	if config.MultiClient {

		if err := os.MkdirAll(filepath.Join(config.ExecDir, "sessions"), 0700); err != nil {
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
			client := GetBotClient(fmt.Sprintf("client%d", idx))
			Workloads[idx] = 0
			clients[int64(idx)] = client
			go func(k string) {
				startBotClient(ctx, client, os.Getenv(k))
			}(key)
		}

	}
}

func getMinWorkloadIndex() int {
	smallest := Workloads[0]
	idx := 0
	for i, workload := range Workloads {
		if workload < smallest {
			smallest = workload
			idx = i
		}
	}
	return idx
}

func GetUploadClient(c *gin.Context) (*telegram.Client, int) {
	if config.MultiClient {
		idx := getMinWorkloadIndex()
		Workloads[idx]++
		return GetBotClient(fmt.Sprintf("client%d", idx)), idx
	} else {
		val, _ := c.Get("jwtUser")
		jwtUser := val.(*types.JWTClaims)
		userId, _ := strconv.ParseInt(jwtUser.Subject, 10, 64)
		client, _ := GetAuthClient(c, jwtUser.TgSession, userId)
		return client, -1
	}
}

func GetDownloadClient(c *gin.Context) (*telegram.Client, int) {
	if config.MultiClient {
		idx := getMinWorkloadIndex()
		Workloads[idx]++
		return clients[int64(idx)], idx
	} else {
		val, _ := c.Get("jwtUser")
		jwtUser := val.(*types.JWTClaims)
		userId, _ := strconv.ParseInt(jwtUser.Subject, 10, 64)
		if client, ok := clients[userId]; ok {
			return client, -1
		}
		client, _ := GetAuthClient(c, jwtUser.TgSession, userId)
		startAuthClient(c, client)
		return client, -1
	}
}
