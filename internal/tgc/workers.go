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
