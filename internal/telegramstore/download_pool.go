package telegramstore

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/gotd/td/tg"

	"github.com/tgdrive/teldrive/v2/internal/cache"
)

const defaultDownloadClientIdleTimeout = 5 * time.Minute

var ErrDownloadClientPoolClosed = errors.New("Telegram download client pool is closed")

type DownloadClientPoolConfig struct {
	Clients      int
	ReadBuffers  int
	ReadParallel int
}

type DownloadClientPool struct {
	runner      Runner
	config      DownloadClientPoolConfig
	globalCache cache.Cacher
	ctx         context.Context
	cancel      context.CancelFunc

	mu      sync.Mutex
	entries map[int64][]*downloadClientEntry
	next    map[int64]uint64
	closed  bool
	done    chan struct{}
}

type downloadClientEntry struct {
	userID   int64
	clientID int64
	ctx      context.Context
	cancel   context.CancelFunc
	ready    chan struct{}
	done     chan struct{}
	api      *tg.Client
	err      error
	refs     int
	lastUsed time.Time
}

func NewDownloadClientPool(runner Runner, config DownloadClientPoolConfig, c cache.Cacher) (*DownloadClientPool, error) {
	if runner == nil {
		return nil, ErrInvalidRequest
	}
	if config.Clients <= 0 {
		config.Clients = 1
	}
	if config.ReadBuffers <= 0 {
		config.ReadBuffers = defaultTelegramReadBuffers
	}
	if config.ReadParallel <= 0 {
		config.ReadParallel = defaultTelegramReadParallel
	}
	ctx, cancel := context.WithCancel(context.Background())
	pool := &DownloadClientPool{
		runner: runner, config: config, globalCache: c, ctx: ctx, cancel: cancel,
		entries: make(map[int64][]*downloadClientEntry), next: make(map[int64]uint64), done: make(chan struct{}),
	}
	go pool.reap()
	return pool, nil
}

func (p *DownloadClientPool) OpenDownloadSession(ctx context.Context, userID int64) (DownloadSession, error) {
	if p == nil || userID <= 0 {
		return nil, ErrInvalidRequest
	}
	entry, created, err := p.lease(userID)
	if err != nil {
		return nil, err
	}
	if created {
		p.start(entry)
	}
	select {
	case <-entry.ready:
		p.mu.Lock()
		api, runErr := entry.api, entry.err
		p.mu.Unlock()
		if api == nil {
			p.release(entry)
			if runErr == nil {
				runErr = ErrClientUnavailable
			}
			return nil, runErr
		}
		return &gotdDownloadSession{
			clientID:             entry.clientID,
			clientFn:             func() (*tg.Client, error) { return p.client(entry, api) },
			closeFn:              func() error { p.release(entry); return nil },
			downloadReadBuffers:  p.config.ReadBuffers,
			downloadReadParallel: p.config.ReadParallel,
			globalCache:          p.globalCache,
		}, nil
	case <-ctx.Done():
		p.release(entry)
		return nil, ctx.Err()
	case <-p.ctx.Done():
		p.release(entry)
		return nil, ErrDownloadClientPoolClosed
	}
}

func (p *DownloadClientPool) lease(userID int64) (*downloadClientEntry, bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil, false, ErrDownloadClientPoolClosed
	}

	entries := p.entries[userID]
	slot := int(p.next[userID] % uint64(p.config.Clients))
	p.next[userID]++
	if slot < len(entries) {
		entry := entries[slot]
		if entry != nil && entry.err == nil && entry.ctx.Err() == nil {
			entry.refs++
			return entry, false, nil
		}
	}

	entryCtx, cancel := context.WithCancel(p.ctx)
	entry := &downloadClientEntry{
		userID: userID, ctx: entryCtx, cancel: cancel, ready: make(chan struct{}), done: make(chan struct{}), refs: 1, lastUsed: time.Now(),
	}
	if slot < len(entries) {
		entries[slot] = entry
	} else {
		entries = append(entries, entry)
	}
	p.entries[userID] = entries
	return entry, true, nil
}

func (p *DownloadClientPool) start(entry *downloadClientEntry) {
	go func() {
		ready := sync.Once{}
		err := runWithConnections(entry.ctx, p.runner, entry.userID, OperationDownload, p.config.ReadParallel, func(runCtx context.Context, api *tg.Client) error {
			p.mu.Lock()
			entry.api = api
			entry.clientID, _ = ClientID(runCtx)
			p.mu.Unlock()
			ready.Do(func() { close(entry.ready) })
			<-runCtx.Done()
			return runCtx.Err()
		})
		p.mu.Lock()
		entry.api = nil
		entry.err = fmt.Errorf("run pooled Telegram download client: %w", err)
		p.removeLocked(entry)
		p.mu.Unlock()
		ready.Do(func() { close(entry.ready) })
		close(entry.done)
	}()
}

func (p *DownloadClientPool) client(entry *downloadClientEntry, expected *tg.Client) (*tg.Client, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed || entry.refs <= 0 || entry.api == nil || entry.api != expected || entry.err != nil {
		return nil, ErrClientUnavailable
	}
	return entry.api, nil
}

func (p *DownloadClientPool) release(entry *downloadClientEntry) {
	p.mu.Lock()
	if entry.refs > 0 {
		entry.refs--
		if entry.refs == 0 {
			entry.lastUsed = time.Now()
		}
	}
	p.mu.Unlock()
}

func (p *DownloadClientPool) reap() {
	interval := defaultDownloadClientIdleTimeout / 2
	ticker := time.NewTicker(interval)
	defer func() {
		ticker.Stop()
		close(p.done)
	}()
	for {
		select {
		case now := <-ticker.C:
			p.reapIdle(now)
		case <-p.ctx.Done():
			return
		}
	}
}

func (p *DownloadClientPool) reapIdle(now time.Time) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, entries := range p.entries {
		for _, entry := range entries {
			if entry != nil && entry.refs == 0 && now.Sub(entry.lastUsed) >= defaultDownloadClientIdleTimeout {
				entry.cancel()
			}
		}
	}
}

func (p *DownloadClientPool) Close(ctx context.Context) error {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	if !p.closed {
		p.closed = true
		p.cancel()
	}
	entries := make([]*downloadClientEntry, 0)
	for _, userEntries := range p.entries {
		for _, entry := range userEntries {
			if entry != nil {
				entries = append(entries, entry)
			}
		}
	}
	p.mu.Unlock()

	for _, entry := range entries {
		select {
		case <-entry.done:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	select {
	case <-p.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (p *DownloadClientPool) removeLocked(target *downloadClientEntry) {
	entries := p.entries[target.userID]
	for i, entry := range entries {
		if entry == target {
			entries[i] = nil
			p.entries[target.userID] = entries
			return
		}
	}
}
