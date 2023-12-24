package tgc

import (
	"context"
	"sync"

	"github.com/gotd/contrib/bg"
	"github.com/gotd/td/telegram"
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

type Client struct {
	Tg     *telegram.Client
	Stop   bg.StopFunc
	Status string
}

type StreamWorker struct {
	mu      sync.Mutex
	bots    map[int64][]string
	clients map[int64][]*Client
	currIdx map[int64]int
}

func (w *StreamWorker) Set(bots []string, channelId int64) {
	w.mu.Lock()
	defer w.mu.Unlock()
	_, ok := w.bots[channelId]
	if !ok {
		w.bots = make(map[int64][]string)
		w.clients = make(map[int64][]*Client)
		w.currIdx = make(map[int64]int)
		w.bots[channelId] = bots
		for _, token := range bots {
			client, _ := BotLogin(context.TODO(), token)
			w.clients[channelId] = append(w.clients[channelId], &Client{Tg: client, Status: "idle"})
		}
		w.currIdx[channelId] = 0
	}

}

func (w *StreamWorker) Next(channelId int64) (*Client, int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	index := w.currIdx[channelId]
	nextClient := w.clients[channelId][index]
	w.currIdx[channelId] = (index + 1) % len(w.clients[channelId])
	if nextClient.Status == "idle" {
		stop, err := bg.Connect(nextClient.Tg)
		if err != nil {
			return nil, 0, err
		}
		nextClient.Stop = stop
		nextClient.Status = "running"
	}
	return nextClient, index, nil
}

func (w *StreamWorker) UserWorker(client *telegram.Client, userId int64) (*Client, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	_, ok := w.clients[userId]

	if !ok {
		w.clients = make(map[int64][]*Client)
		w.clients[userId] = append(w.clients[userId], &Client{Tg: client, Status: "idle"})
	}
	nextClient := w.clients[userId][0]
	if nextClient.Status == "idle" {
		stop, err := bg.Connect(nextClient.Tg)
		if err != nil {
			return nil, err
		}
		nextClient.Stop = stop
		nextClient.Status = "running"
	}
	return nextClient, nil
}
