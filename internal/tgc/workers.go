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
	mu      sync.Mutex
	bots    map[int64][]string
	currIdx map[int64]int
}

func (w *UploadWorker) Set(bots []string, channelId int64) {
	w.mu.Lock()
	defer w.mu.Unlock()
	_, ok := w.bots[channelId]
	if !ok {
		w.bots = make(map[int64][]string)
		w.currIdx = make(map[int64]int)
		w.bots[channelId] = bots
		w.currIdx[channelId] = 0
	}
}

func (w *UploadWorker) Next(channelId int64) (string, int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	index := w.currIdx[channelId]
	w.currIdx[channelId] = (index + 1) % len(w.bots[channelId])
	return w.bots[channelId][index], index
}

func NewUploadWorker() *UploadWorker {
	return &UploadWorker{}
}

type Client struct {
	Tg          *telegram.Client
	Stop        StopFunc
	Status      string
	UserId      string
	lastUsed    time.Time
	connections int
}

type StreamWorker struct {
	mu          sync.Mutex
	clients     map[string]*Client
	currIdx     map[int64]int
	channelBots map[int64][]string
	cnf         *config.TGConfig
	kv          kv.KV
	ctx         context.Context
	logger      *zap.SugaredLogger
}

func (w *StreamWorker) Set(bots []string, channelId int64) {

	w.mu.Lock()
	defer w.mu.Unlock()
	_, ok := w.channelBots[channelId]
	if !ok {
		w.channelBots[channelId] = bots
		w.currIdx[channelId] = 0
	}

}

func (w *StreamWorker) Next(channelId int64) (*Client, int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	index := w.currIdx[channelId]
	token := w.channelBots[channelId][index]
	userId := strings.Split(token, ":")[0]
	client, ok := w.clients[userId]
	if !ok || (client.Status == "idle" && client.Stop == nil) {
		middlewares := Middlewares(w.cnf, 5)
		tgClient, _ := BotClient(w.ctx, w.kv, w.cnf, token, middlewares...)
		client = &Client{Tg: tgClient, Status: "idle", UserId: userId}
		w.clients[userId] = client
		stop, err := Connect(client.Tg, WithBotToken(token))
		if err != nil {
			return nil, 0, err
		}
		client.Stop = stop
		w.logger.Debug("started bg client: ", client.UserId)
	}
	w.currIdx[channelId] = (index + 1) % len(w.channelBots[channelId])
	client.lastUsed = time.Now()
	if client.connections == 0 {
		client.Status = "serving"
	}
	client.connections++
	return client, index, nil
}

func (w *StreamWorker) Release(client *Client) {
	w.mu.Lock()
	defer w.mu.Unlock()
	client.connections--
	if client.connections == 0 {
		client.Status = "running"
	}
}

func (w *StreamWorker) UserWorker(session string, userId int64) (*Client, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	id := strconv.FormatInt(userId, 10)
	client, ok := w.clients[id]
	if !ok || (client.Status == "idle" && client.Stop == nil) {
		middlewares := Middlewares(w.cnf, 5)
		tgClient, _ := AuthClient(w.ctx, w.cnf, session, middlewares...)
		client = &Client{Tg: tgClient, Status: "idle", UserId: id}
		w.clients[id] = client
		stop, err := Connect(client.Tg, WithContext(w.ctx))
		if err != nil {
			return nil, err
		}
		client.Stop = stop
		w.logger.Debug("started bg client: ", client.UserId)
	}
	client.lastUsed = time.Now()
	if client.connections == 0 {
		client.Status = "serving"
	}
	client.connections++
	return client, nil
}

func (w *StreamWorker) startIdleClientMonitor() {
	ticker := time.NewTicker(w.cnf.BgBotsCheckInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			w.mu.Lock()
			for _, client := range w.clients {
				if client.Status == "running" && time.Since(client.lastUsed) > w.cnf.BgBotsTimeout {
					if client.Stop != nil {
						client.Stop()
						client.Stop = nil
						client.Status = "idle"
						w.logger.Debug("stopped bg client: ", client.UserId)
					}
				}
			}
			w.mu.Unlock()
		case <-w.ctx.Done():
			return
		}
	}

}

func NewStreamWorker(ctx context.Context) func(cnf *config.Config, kv kv.KV) *StreamWorker {
	return func(cnf *config.Config, kv kv.KV) *StreamWorker {
		worker := &StreamWorker{cnf: &cnf.TG, kv: kv, ctx: ctx,
			clients: make(map[string]*Client), currIdx: make(map[int64]int),
			channelBots: make(map[int64][]string), logger: logging.FromContext(ctx)}
		go worker.startIdleClientMonitor()
		return worker
	}
}
