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

	"github.com/gotd/contrib/bg"
	"github.com/gotd/contrib/middleware/ratelimit"
	"github.com/gotd/td/telegram"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"golang.org/x/time/rate"
)

type Client struct {
	Tg       *telegram.Client
	Token    string
	Workload int
}

var clients map[int]*Client

func initClient(appID int, appHash, clientName, sessionDir string) *telegram.Client {

	sessionStorage := &telegram.FileSessionStorage{
		Path: filepath.Join(sessionDir, clientName+".json"),
	}

	options := telegram.Options{
		SessionStorage: sessionStorage,
		Middlewares: []telegram.Middleware{
			ratelimit.New(rate.Every(time.Millisecond*100), 5),
		},
	}

	client := telegram.NewClient(appID, appHash, options)

	return client

}

func startClient(ctx context.Context, client *Client, lg *zap.Logger) (bg.StopFunc, error) {

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

	lg.Info("started Client", zap.String("user", tguser.Username))
	return stop, nil
}

func StartClients() {

	appID, err := strconv.Atoi(os.Getenv("APP_ID"))

	if err != nil {
		return
	}

	appHash := os.Getenv("APP_HASH")

	if appHash == "" {
		return
	}

	sessionDir := "sessions"

	if err := os.MkdirAll(sessionDir, 0700); err != nil {
		return
	}

	lg, _ := zap.NewDevelopment(zap.IncreaseLevel(zapcore.InfoLevel), zap.AddStacktrace(zapcore.FatalLevel))

	var keysToSort []string

	for _, e := range os.Environ() {
		if strings.HasPrefix(e, "MULTI_TOKEN") {
			if i := strings.Index(e, "="); i >= 0 {
				keysToSort = append(keysToSort, e[:i])
			}
		}
	}

	sort.Strings(keysToSort)

	clients = make(map[int]*Client)

	for idx, key := range keysToSort {
		client := initClient(appID, appHash, fmt.Sprintf("client%d", idx), sessionDir)
		clients[idx] = &Client{Tg: client, Token: os.Getenv(key)}
	}

	ctx := context.Background()

	for _, client := range clients {
		go startClient(ctx, client, lg)
	}

}

func GetTgClient() *Client {
	smallest := clients[0]
	for _, client := range clients {
		if client.Workload < smallest.Workload {
			smallest = client
		}
	}
	return smallest
}
