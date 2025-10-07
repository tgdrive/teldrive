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

func (w *BotWorker) Set(bots []string, userID int64) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if _, ok := w.bots[userID]; ok {
		return
	}
	w.bots[userID] = bots
	w.currIdx[userID] = 0
}

func (w *BotWorker) Next(userID int64) (string, int) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	bots := w.bots[userID]
	index := w.currIdx[userID]
	w.currIdx[userID] = (index + 1) % len(bots)
	return bots[index], index
}
