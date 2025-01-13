package tgc

import (
	"sync"
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

func (w *BotWorker) Set(bots []string, channelId int64) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if _, ok := w.bots[channelId]; ok {
		return
	}
	w.bots[channelId] = bots
	w.currIdx[channelId] = 0
}

func (w *BotWorker) Next(channelId int64) (string, int) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	bots := w.bots[channelId]
	index := w.currIdx[channelId]
	w.currIdx[channelId] = (index + 1) % len(bots)
	return bots[index], index
}
