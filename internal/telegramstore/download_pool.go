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

// AccountLease reserves active load on the Telegram account chosen for one unit
// of work until Release is called.
type AccountLease interface {
	// AccountID identifies the Telegram account (user or bot) behind the lease.
	AccountID() int64
	// RunPooled runs fn on a client bound to the leased account. The client may
	// keep running after Release; only leased work counts as account load, not
	// the idle client.
	RunPooled(ctx context.Context, connections int, fn func(context.Context, *tg.Client) error) error
	// Release returns the reserved load. It is safe to call more than once.
	Release()
}

// AccountLeaser chooses the Telegram account that serves the next unit of work.
type AccountLeaser interface {
	LeaseAccount(ctx context.Context, userID int64, operation Operation, connections int) (AccountLease, error)
}

type DownloadClientPoolConfig struct {
	ReadBuffers  int
	ReadParallel int
}

// DownloadClientPool keeps one authenticated download client warm per user and
// Telegram account. Each download session leases an account from the
// AccountLeaser, so account selection happens per session, and reuses the warm
// client for that account or starts one.
type DownloadClientPool struct {
	leaser      AccountLeaser
	config      DownloadClientPoolConfig
	globalCache cache.Cacher
	ctx         context.Context
	cancel      context.CancelFunc

	mu      sync.Mutex
	entries map[downloadClientKey]*downloadClientEntry
	closed  bool
	done    chan struct{}
}

type downloadClientKey struct {
	userID    int64
	accountID int64
}

type downloadClientEntry struct {
	key      downloadClientKey
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

func NewDownloadClientPool(leaser AccountLeaser, config DownloadClientPoolConfig, c cache.Cacher) (*DownloadClientPool, error) {
	if leaser == nil {
		return nil, ErrInvalidRequest
	}
	if config.ReadBuffers <= 0 {
		config.ReadBuffers = defaultTelegramReadBuffers
	}
	if config.ReadParallel <= 0 {
		config.ReadParallel = defaultTelegramReadParallel
	}
	ctx, cancel := context.WithCancel(context.Background())
	pool := &DownloadClientPool{
		leaser: leaser, config: config, globalCache: c, ctx: ctx, cancel: cancel,
		entries: make(map[downloadClientKey]*downloadClientEntry), done: make(chan struct{}),
	}
	go pool.reap()
	return pool, nil
}

// OpenDownloadSession leases an account and returns a session on its warm
// client. When the leased account turns out to be revoked before the client is
// ready, the leaser has already dropped it, so another account is leased.
func (p *DownloadClientPool) OpenDownloadSession(ctx context.Context, userID int64) (DownloadSession, error) {
	if p == nil || userID <= 0 {
		return nil, ErrInvalidRequest
	}
	for {
		session, err := p.openDownloadSession(ctx, userID)
		if !errors.Is(err, ErrAccountRevoked) {
			return session, err
		}
	}
}

func (p *DownloadClientPool) openDownloadSession(ctx context.Context, userID int64) (DownloadSession, error) {
	lease, err := p.leaser.LeaseAccount(ctx, userID, OperationDownload, p.config.ReadParallel)
	if err != nil {
		return nil, err
	}
	entry, created, err := p.lease(downloadClientKey{userID: userID, accountID: lease.AccountID()})
	if err != nil {
		lease.Release()
		return nil, err
	}
	if created {
		p.start(entry, lease)
	}
	release := func() {
		p.release(entry)
		lease.Release()
	}
	select {
	case <-entry.ready:
		p.mu.Lock()
		api, runErr := entry.api, entry.err
		p.mu.Unlock()
		if api == nil {
			release()
			if runErr == nil {
				runErr = ErrClientUnavailable
			}
			return nil, runErr
		}
		return &gotdDownloadSession{
			clientID:             entry.clientID,
			clientFn:             func() (*tg.Client, error) { return p.client(entry, api) },
			closeFn:              func() error { release(); return nil },
			downloadReadBuffers:  p.config.ReadBuffers,
			downloadReadParallel: p.config.ReadParallel,
			globalCache:          p.globalCache,
		}, nil
	case <-ctx.Done():
		release()
		return nil, ctx.Err()
	case <-p.ctx.Done():
		release()
		return nil, ErrDownloadClientPoolClosed
	}
}

// lease returns the live warm client for key, or registers a new one that the
// caller must start.
func (p *DownloadClientPool) lease(key downloadClientKey) (*downloadClientEntry, bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil, false, ErrDownloadClientPoolClosed
	}
	if entry := p.entries[key]; entry != nil && entry.err == nil && entry.ctx.Err() == nil {
		entry.refs++
		return entry, false, nil
	}
	entryCtx, cancel := context.WithCancel(p.ctx)
	entry := &downloadClientEntry{
		key: key, ctx: entryCtx, cancel: cancel, ready: make(chan struct{}), done: make(chan struct{}), refs: 1, lastUsed: time.Now(),
	}
	p.entries[key] = entry
	return entry, true, nil
}

// start runs the warm client through the lease that created it. The client
// outlives that lease; later sessions take their own leases on the account.
func (p *DownloadClientPool) start(entry *downloadClientEntry, lease AccountLease) {
	go func() {
		ready := sync.Once{}
		err := lease.RunPooled(entry.ctx, p.config.ReadParallel, func(runCtx context.Context, api *tg.Client) error {
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
		if p.entries[entry.key] == entry {
			delete(p.entries, entry.key)
		}
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
	for _, entry := range p.entries {
		if entry.refs == 0 && now.Sub(entry.lastUsed) >= defaultDownloadClientIdleTimeout {
			entry.cancel()
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
	entries := make([]*downloadClientEntry, 0, len(p.entries))
	for _, entry := range p.entries {
		entries = append(entries, entry)
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
