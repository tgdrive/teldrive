package telegramstore

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/gotd/td/tg"
)

var (
	ErrDownloadClientPoolClosed = errors.New("Telegram download client pool is closed")
	ErrDownloadClientPoolBusy   = errors.New("Telegram download client pool is busy")
)

type DownloadClientPoolConfig struct {
	ClientsPerUser int
	MaxClients     int
	MaxSessions    int
	ReadBuffers    int
	ReadParallel   int
	IdleTimeout    time.Duration
	AcquireTimeout time.Duration
}

type DownloadClientPool struct {
	runner Runner
	config DownloadClientPoolConfig
	ctx    context.Context
	cancel context.CancelFunc

	mu      sync.Mutex
	entries map[int64][]*downloadClientEntry
	total   int
	closed  bool
	notify  chan struct{}
	done    chan struct{}
}

type downloadClientEntry struct {
	userID        int64
	ctx           context.Context
	cancel        context.CancelFunc
	ready         chan struct{}
	done          chan struct{}
	api           *tg.Client
	locationCache *documentLocationCache
	err           error
	refs          int
	lastUsed      time.Time
}

func NewDownloadClientPool(runner Runner, config DownloadClientPoolConfig) (*DownloadClientPool, error) {
	if config.ReadBuffers <= 0 {
		config.ReadBuffers = defaultTelegramReadBuffers
	}
	if config.ReadParallel <= 0 {
		config.ReadParallel = defaultTelegramReadParallel
	}
	if runner == nil || config.ClientsPerUser < 1 || config.MaxClients < 1 || config.MaxSessions < 1 ||
		config.ClientsPerUser > config.MaxClients || config.IdleTimeout <= 0 || config.AcquireTimeout <= 0 {
		return nil, ErrInvalidRequest
	}
	ctx, cancel := context.WithCancel(context.Background())
	pool := &DownloadClientPool{
		runner: runner, config: config, ctx: ctx, cancel: cancel,
		entries: make(map[int64][]*downloadClientEntry), notify: make(chan struct{}, 1), done: make(chan struct{}),
	}
	go pool.reap()
	return pool, nil
}

func (p *DownloadClientPool) OpenDownloadSession(ctx context.Context, userID int64) (DownloadSession, error) {
	if p == nil || userID <= 0 {
		return nil, ErrInvalidRequest
	}
	acquireCtx, cancel := context.WithTimeout(ctx, p.config.AcquireTimeout)
	defer cancel()

	for {
		entry, created, err := p.reserve(userID)
		if err != nil {
			return nil, err
		}
		if entry != nil {
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
					clientFn:             func() (*tg.Client, error) { return p.client(entry, api) },
					closeFn:              func() error { p.release(entry); return nil },
					downloadReadBuffers:  p.config.ReadBuffers,
					downloadReadParallel: p.config.ReadParallel,
					locationCache:        entry.locationCache,
				}, nil
			case <-acquireCtx.Done():
				p.release(entry)
				return nil, errors.Join(ErrDownloadClientPoolBusy, acquireCtx.Err())
			}
		}

		select {
		case <-p.notify:
		case <-acquireCtx.Done():
			return nil, errors.Join(ErrDownloadClientPoolBusy, acquireCtx.Err())
		}
	}
}

func (p *DownloadClientPool) reserve(userID int64) (*downloadClientEntry, bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil, false, ErrDownloadClientPoolClosed
	}
	var selected *downloadClientEntry
	for _, entry := range p.entries[userID] {
		if entry.api != nil && entry.err == nil && entry.ctx.Err() == nil && entry.refs < p.config.MaxSessions &&
			(selected == nil || entry.refs < selected.refs) {
			selected = entry
		}
	}
	if selected != nil {
		selected.refs++
		return selected, false, nil
	}
	if len(p.entries[userID]) >= p.config.ClientsPerUser {
		return nil, false, nil
	}
	if p.total >= p.config.MaxClients {
		var oldest *downloadClientEntry
		for _, entries := range p.entries {
			for _, entry := range entries {
				if entry.refs == 0 && entry.ctx.Err() == nil && (oldest == nil || entry.lastUsed.Before(oldest.lastUsed)) {
					oldest = entry
				}
			}
		}
		if oldest != nil {
			oldest.cancel()
		}
		return nil, false, nil
	}
	entryCtx, cancel := context.WithCancel(p.ctx)
	entry := &downloadClientEntry{
		userID: userID, ctx: entryCtx, cancel: cancel, ready: make(chan struct{}), done: make(chan struct{}),
		refs: 1, lastUsed: time.Now(), locationCache: newDocumentLocationCache(),
	}
	p.entries[userID] = append(p.entries[userID], entry)
	p.total++
	return entry, true, nil
}

func (p *DownloadClientPool) start(entry *downloadClientEntry) {
	go func() {
		ready := sync.Once{}
		err := runWithConnections(entry.ctx, p.runner, entry.userID, OperationDownload, p.config.ReadParallel, func(runCtx context.Context, api *tg.Client) error {
			p.mu.Lock()
			entry.api = api
			p.mu.Unlock()
			ready.Do(func() { close(entry.ready) })
			p.signal()
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
		p.signal()
	}()
}

func (p *DownloadClientPool) client(entry *downloadClientEntry, expected *tg.Client) (*tg.Client, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed || entry.api == nil || entry.api != expected || entry.err != nil {
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
	p.signal()
}

func (p *DownloadClientPool) reap() {
	interval := p.config.IdleTimeout / 2
	if interval > time.Minute {
		interval = time.Minute
	}
	if interval < 10*time.Millisecond {
		interval = 10 * time.Millisecond
	}
	ticker := time.NewTicker(interval)
	defer func() {
		ticker.Stop()
		close(p.done)
	}()
	for {
		select {
		case now := <-ticker.C:
			p.mu.Lock()
			for _, entries := range p.entries {
				for _, entry := range entries {
					if entry.refs == 0 && now.Sub(entry.lastUsed) >= p.config.IdleTimeout {
						entry.cancel()
					}
				}
			}
			p.mu.Unlock()
		case <-p.ctx.Done():
			return
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
	entries := make([]*downloadClientEntry, 0, p.total)
	for _, userEntries := range p.entries {
		entries = append(entries, userEntries...)
	}
	p.mu.Unlock()
	p.signal()

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
		if entry != target {
			continue
		}
		entries[i] = entries[len(entries)-1]
		entries = entries[:len(entries)-1]
		p.total--
		break
	}
	if len(entries) == 0 {
		delete(p.entries, target.userID)
	} else {
		p.entries[target.userID] = entries
	}
}

func (p *DownloadClientPool) signal() {
	select {
	case p.notify <- struct{}{}:
	default:
	}
}
