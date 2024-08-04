package tgc

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/divyam234/teldrive/internal/config"
	"github.com/divyam234/teldrive/internal/kv"
	"github.com/divyam234/teldrive/internal/logging"
	"github.com/gotd/td/telegram"
	"go.uber.org/zap"
)

type UploadWorker struct {
	mu      sync.RWMutex
	bots    map[int64][]string
	currIdx map[int64]int
}

func NewUploadWorker() *UploadWorker {
	return &UploadWorker{
		bots:    make(map[int64][]string),
		currIdx: make(map[int64]int),
	}
}

func (w *UploadWorker) Set(bots []string, channelID int64) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.bots[channelID] = bots
	w.currIdx[channelID] = 0
}

func (w *UploadWorker) Next(channelID int64) (string, int) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	bots := w.bots[channelID]
	index := w.currIdx[channelID]
	w.currIdx[channelID] = (index + 1) % len(bots)
	return bots[index], index
}

type Client struct {
	Tg          *telegram.Client
	Stop        StopFunc
	Status      string
	UserID      string
	LastUsed    time.Time
	Connections int
}

type StreamWorker struct {
	mu          sync.RWMutex
	clients     map[string]*Client
	currIdx     map[int64]int
	channelBots map[int64][]string
	cnf         *config.TGConfig
	kv          kv.KV
	ctx         context.Context
	logger      *zap.SugaredLogger
}

func NewStreamWorker(ctx context.Context) func(cnf *config.Config, kv kv.KV) *StreamWorker {
	return func(cnf *config.Config, kv kv.KV) *StreamWorker {
		worker := &StreamWorker{
			cnf:         &cnf.TG,
			kv:          kv,
			ctx:         ctx,
			clients:     make(map[string]*Client),
			currIdx:     make(map[int64]int),
			channelBots: make(map[int64][]string),
			logger:      logging.FromContext(ctx),
		}
		go worker.startIdleClientMonitor()
		return worker
	}
}

func (w *StreamWorker) Set(bots []string, channelID int64) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.channelBots[channelID] = bots
	w.currIdx[channelID] = 0
}

func (w *StreamWorker) Next(channelID int64) (*Client, int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	bots := w.channelBots[channelID]
	index := w.currIdx[channelID]
	token := bots[index]
	userID := strings.Split(token, ":")[0]

	client, err := w.getOrCreateClient(userID, token)
	if err != nil {
		return nil, 0, err
	}

	w.currIdx[channelID] = (index + 1) % len(bots)
	client.LastUsed = time.Now()
	client.Connections++
	if client.Connections == 1 {
		client.Status = "serving"
	}

	return client, index, nil
}

func (w *StreamWorker) getOrCreateClient(userID, token string) (*Client, error) {
	client, ok := w.clients[userID]
	if !ok || (client.Status == "idle" && client.Stop == nil) {
		middlewares := Middlewares(w.cnf, 5)
		tgClient, _ := BotClient(w.ctx, w.kv, w.cnf, token, middlewares...)
		client = &Client{Tg: tgClient, Status: "idle", UserID: userID}
		w.clients[userID] = client

		stop, err := Connect(client.Tg, WithBotToken(token))
		if err != nil {
			return nil, err
		}
		client.Stop = stop
		w.logger.Debug("started bg client: ", client.UserID)
	}
	return client, nil
}

func (w *StreamWorker) Release(client *Client) {
	w.mu.Lock()
	defer w.mu.Unlock()
	client.Connections--
	if client.Connections == 0 {
		client.Status = "running"
	}
}

func (w *StreamWorker) UserWorker(session string, userID int64) (*Client, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	id := strconv.FormatInt(userID, 10)
	client, ok := w.clients[id]
	if !ok || (client.Status == "idle" && client.Stop == nil) {
		middlewares := Middlewares(w.cnf, 5)
		tgClient, _ := AuthClient(w.ctx, w.cnf, session, middlewares...)
		client = &Client{Tg: tgClient, Status: "idle", UserID: id}
		w.clients[id] = client

		stop, err := Connect(client.Tg, WithContext(w.ctx))
		if err != nil {
			return nil, err
		}
		client.Stop = stop
		w.logger.Debug("started bg client: ", client.UserID)
	}

	client.LastUsed = time.Now()
	client.Connections++
	if client.Connections == 1 {
		client.Status = "serving"
	}

	return client, nil
}

func (w *StreamWorker) startIdleClientMonitor() {
	ticker := time.NewTicker(w.cnf.BgBotsCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			w.checkIdleClients()
		case <-w.ctx.Done():
			return
		}
	}
}

func (w *StreamWorker) checkIdleClients() {
	w.mu.Lock()
	defer w.mu.Unlock()

	for _, client := range w.clients {
		if client.Status == "running" && time.Since(client.LastUsed) > w.cnf.BgBotsTimeout {
			if client.Stop != nil {
				client.Stop()
				client.Stop = nil
				client.Status = "idle"
				w.logger.Debug("stopped bg client: ", client.UserID)
			}
		}
	}
}
