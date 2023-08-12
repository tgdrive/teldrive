package utils

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gotd/contrib/bg"
	"github.com/gotd/contrib/middleware/ratelimit"
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

func getBotClient(appID int, appHash, clientName, sessionDir string) *telegram.Client {

	sessionStorage := &telegram.FileSessionStorage{
		Path: filepath.Join(sessionDir, clientName+".json"),
	}
	options := telegram.Options{
		SessionStorage: sessionStorage,
		// Middlewares: []telegram.Middleware{
		// 	ratelimit.New(rate.Every(time.Millisecond*100), 5),
		// },
		NoUpdates: true,
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

func GetAuthClient(sessionStr string, userId int) (*Client, error, bg.StopFunc) {

	if client, ok := clients[userId]; ok {
		return client, nil, nil
	}

	ctx := context.Background()

	data, err := session.TelethonSession(sessionStr)

	if err != nil {
		return nil, err, nil
	}

	var (
		storage = new(session.StorageMemory)
		loader  = session.Loader{Storage: storage}
	)

	if err := loader.Save(ctx, data); err != nil {
		return nil, err, nil
	}

	client := telegram.NewClient(config.AppId, config.AppHash, telegram.Options{
		SessionStorage: storage,
		Middlewares: []telegram.Middleware{
			ratelimit.New(rate.Every(time.Millisecond*100), 5),
		},
		NoUpdates: true,
	})

	stop, err := bg.Connect(client)

	if err != nil {
		return nil, err, nil
	}

	tguser, err := client.Self(ctx)

	if err != nil {
		return nil, err, nil
	}

	Logger.Info("started Client", zap.String("user", tguser.Username))

	tgClient := &Client{Tg: client}

	clients[int(tguser.GetID())] = tgClient

	return tgClient, nil, stop
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

func StopClient(stop bg.StopFunc, key int) {
	stop()
	delete(clients, key)
}
