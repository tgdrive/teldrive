package tgc

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/gotd/td/telegram"
	"github.com/tgdrive/teldrive/internal/config"
	"github.com/tgdrive/teldrive/internal/kv"
	"go.uber.org/zap"
)

type BotWorker struct {
	mu      sync.RWMutex
	bots    map[int64][]string
	currIdx map[int64]int
}

func NewBotWorker() *BotWorker {
	return &BotWorker{
		bots:    make(map[int64][]string),
		currIdx: make(map[int64]int),
	}
}

func (w *BotWorker) Set(bots []string, channelID int64) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if _, ok := w.bots[channelID]; ok {
		return
	}
	w.bots[channelID] = bots
	w.currIdx[channelID] = 0
}

func (w *BotWorker) Next(channelID int64) (string, int) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	bots := w.bots[channelID]
	index := w.currIdx[channelID]
	w.currIdx[channelID] = (index + 1) % len(bots)
	return bots[index], index
}

type ClientStatus int

const (
	StatusIdle ClientStatus = iota
	StatusBusy
)

type Client struct {
	Tg     *telegram.Client
	Stop   StopFunc
	Status ClientStatus
	UserID string
}

type StreamWorker struct {
	mu            sync.RWMutex
	clients       map[string]*Client
	currIdx       map[int64]int
	channelBots   map[int64][]string
	cnf           *config.TGConfig
	kv            kv.KV
	ctx           context.Context
	logger        *zap.SugaredLogger
	activeStreams int
	cancel        context.CancelFunc
}

func NewStreamWorker(cnf *config.Config, kv kv.KV, logger *zap.SugaredLogger) *StreamWorker {
	ctx, cancel := context.WithCancel(context.Background())
	worker := &StreamWorker{
		cnf:         &cnf.TG,
		kv:          kv,
		ctx:         ctx,
		clients:     make(map[string]*Client),
		currIdx:     make(map[int64]int),
		channelBots: make(map[int64][]string),
		logger:      logger,
		cancel:      cancel,
	}
	go worker.startIdleClientMonitor()
	return worker

}

func (w *StreamWorker) Set(bots []string, channelID int64) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.channelBots[channelID] = bots
	w.currIdx[channelID] = 0
}

func (w *StreamWorker) Next(channelID int64) (*Client, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	bots := w.channelBots[channelID]
	index := w.currIdx[channelID]
	token := bots[index]
	userID := strings.Split(token, ":")[0]

	client, err := w.getOrCreateClient(userID, token)
	if err != nil {
		return nil, err
	}

	w.currIdx[channelID] = (index + 1) % len(bots)

	return client, nil
}

func (w *StreamWorker) IncActiveStream() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.activeStreams++
	return nil
}

func (w *StreamWorker) DecActiveStreams() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.activeStreams == 0 {
		return nil
	}
	w.activeStreams--
	return nil
}

func (w *StreamWorker) getOrCreateClient(userID, token string) (*Client, error) {
	client, ok := w.clients[userID]
	if !ok || (client.Status == StatusIdle && client.Stop == nil) {
		middlewares := Middlewares(w.cnf, 5)
		tgClient, _ := BotClient(w.ctx, w.kv, w.cnf, token, middlewares...)
		client = &Client{Tg: tgClient, Status: StatusIdle, UserID: userID}
		w.clients[userID] = client
		stop, err := Connect(client.Tg, WithBotToken(token))
		if err != nil {
			return nil, err
		}
		client.Stop = stop
		client.Status = StatusBusy
		w.logger.Debug("started bg client: ", userID)
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
	if w.activeStreams == 0 {
		for _, client := range w.clients {
			if client.Status == StatusBusy && client.Stop != nil {
				client.Stop()
				client.Stop = nil
				client.Tg = nil
				client.Status = StatusIdle
				w.logger.Debug("stopped bg client: ", client.UserID)
			}
		}
	}

}
