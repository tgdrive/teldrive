package tgc

import (
	"sync"
)

type BotWorkers struct {
	sync.Mutex
	bots  []string
	index int
}

func (w *BotWorkers) Set(bots []string) {
	w.Lock()
	defer w.Unlock()
	if len(w.bots) == 0 {
		w.bots = bots
	}
}

func (w *BotWorkers) Next() string {
	w.Lock()
	defer w.Unlock()
	item := w.bots[w.index]
	w.index = (w.index + 1) % len(w.bots)
	return item
}

var Workers = &BotWorkers{}
