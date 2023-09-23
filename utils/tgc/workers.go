package tgc

import (
	"sync"

	"github.com/gotd/contrib/bg"
	"github.com/gotd/td/telegram"
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

type Client struct {
	Tg     *telegram.Client
	Stop   bg.StopFunc
	Status string
}

type streamWorkers struct {
	sync.Mutex
	bots    []string
	clients []*Client
	index   int
}

func (w *streamWorkers) Set(bots []string) {
	w.Lock()
	defer w.Unlock()
	if len(w.clients) == 0 {
		w.bots = bots
		for _, token := range bots {
			client, _ := BotLogin(token)
			w.clients = append(w.clients, &Client{Tg: client, Status: "idle"})
		}
	}
}

func (w *streamWorkers) Next() (*Client, error) {
	w.Lock()
	defer w.Unlock()
	w.index = (w.index + 1) % len(w.clients)
	if w.clients[w.index].Status == "idle" {
		stop, err := bg.Connect(w.clients[w.index].Tg)
		if err != nil {
			return nil, err
		}
		w.clients[w.index].Stop = stop
		w.clients[w.index].Status = "running"
	}
	return w.clients[w.index], nil
}

var StreamWorkers = &streamWorkers{}
