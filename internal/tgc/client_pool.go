package tgc

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
	"github.com/tgdrive/teldrive/internal/cache"
	"github.com/tgdrive/teldrive/internal/config"
	"github.com/tgdrive/teldrive/internal/logging"
	"github.com/tgdrive/teldrive/internal/pool"
	"github.com/tgdrive/teldrive/pkg/models"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// RoutingStrategy defines how to select a client from the pool.
type RoutingStrategy string

const (
	RoundRobin       RoutingStrategy = "round_robin"
	LeastConnections RoutingStrategy = "least_connections"
)

// PooledClient wraps a telegram.Client with connection tracking.
type PooledClient struct {
	Client      *telegram.Client
	Connections int64
	LastUsed    time.Time
	Key         string
	stop        func() error
	Creating    int32 // atomic: 1 = being created
	IsReady     int32 // atomic: 1 = ready for use
	TgClient    *tg.Client
	Close       func() error
}

// ClientPool manages a pool of background telegram clients with connection tracking and idle timeout.
type ClientPool struct {
	clients     sync.Map
	db          *gorm.DB
	cache       cache.Cacher
	cnf         *config.TGConfig
	logger      *zap.Logger
	idleTimeout time.Duration
	closeChan   chan struct{}
	wg          sync.WaitGroup
	totalConns  int64
	strategy    RoutingStrategy
	currentIdx  map[string]int
	idxMu       sync.RWMutex
}

// PoolStats contains pool statistics.
type PoolStats struct {
	TotalClients int
	TotalConns   int64
	ClientStats  map[string]int64
	Strategy     RoutingStrategy
}

// NewClientPool creates a new ClientPool with configurable routing strategy.
func NewClientPool(db *gorm.DB, cache cache.Cacher, cnf *config.TGConfig) *ClientPool {
	timeout := 30 * time.Minute
	if cnf != nil && cnf.PoolIdleTimeout > 0 {
		timeout = cnf.PoolIdleTimeout
	}

	strategy := LeastConnections
	if cnf != nil && cnf.PoolRoutingStrategy == "round_robin" {
		strategy = RoundRobin
	}

	pool := &ClientPool{
		db:          db,
		cache:       cache,
		cnf:         cnf,
		logger:      logging.Component("TG"),
		idleTimeout: timeout,
		closeChan:   make(chan struct{}),
		strategy:    strategy,
		currentIdx:  make(map[string]int),
	}

	pool.wg.Add(1)
	go pool.idleChecker()

	return pool
}

func (p *ClientPool) selectClient(clients []*PooledClient) *PooledClient {
	if len(clients) == 0 {
		return nil
	}

	switch p.strategy {
	case RoundRobin:
		key := fmt.Sprintf("rr:%d", len(clients))
		p.idxMu.Lock()
		idx := p.currentIdx[key]
		p.currentIdx[key] = (idx + 1) % len(clients)
		p.idxMu.Unlock()
		return clients[idx]
	case LeastConnections:
		selected := clients[0]
		minConns := atomic.LoadInt64(&selected.Connections)
		for _, c := range clients[1:] {
			conns := atomic.LoadInt64(&c.Connections)
			if conns < minConns {
				selected = c
				minConns = conns
			}
		}
		return selected
	default:
		return clients[0]
	}
}

func (p *ClientPool) Acquire(key string) {
	iface, ok := p.clients.Load(key)
	if ok {
		pc := iface.(*PooledClient)
		atomic.AddInt64(&pc.Connections, 1)
		atomic.AddInt64(&p.totalConns, 1)
		pc.LastUsed = time.Now()
	}
}

func (p *ClientPool) Release(key string) {
	iface, ok := p.clients.Load(key)
	if ok {
		pc := iface.(*PooledClient)
		atomic.AddInt64(&pc.Connections, -1)
		atomic.AddInt64(&p.totalConns, -1)
		pc.LastUsed = time.Now()
	}
}

// GetUserClient returns a ready-to-use telegram.Client for a user session.
func (p *ClientPool) GetUserClient(session *models.Session) (*tg.Client, string, error) {
	key := fmt.Sprintf("user:%d", session.UserId)

	iface, ok := p.clients.Load(key)
	if ok {
		pc := iface.(*PooledClient)
		if atomic.LoadInt32(&pc.IsReady) == 1 {
			p.logger.Debug("client.reuse", zap.String("key", key))
			p.Acquire(key)
			return pc.TgClient, key, nil
		}
	}

	// Try to create if not exists
	newPC := &PooledClient{
		Key:      key,
		LastUsed: time.Now(),
	}
	actual, loaded := p.clients.LoadOrStore(key, newPC)
	pc := actual.(*PooledClient)

	if loaded && atomic.LoadInt32(&pc.IsReady) == 1 {
		p.Acquire(key)
		return pc.TgClient, key, nil
	}

	// Use CAS to claim creation
	if !atomic.CompareAndSwapInt32(&pc.Creating, 0, 1) {
		// Another goroutine is creating, wait and retry
		for atomic.LoadInt32(&pc.Creating) == 1 {
			time.Sleep(10 * time.Millisecond)
		}
		return p.GetUserClient(session)
	}

	// We claimed creation
	defer atomic.StoreInt32(&pc.Creating, 0)

	// Double check after claiming
	if atomic.LoadInt32(&pc.IsReady) == 1 {
		p.Acquire(pc.Key)
		return pc.TgClient, pc.Key, nil
	}
	err := p.createUserClient(key, session)
	if err != nil {
		return nil, "", err
	}
	return pc.TgClient, pc.Key, nil
}

// GetBotClient returns a ready-to-use telegram.Client for bots using routing strategy.
// Each user gets isolated bot clients - rate limits are per user-bot combination.
func (p *ClientPool) GetBotClient(userID int64, bots []string) (*tg.Client, string, error) {
	if len(bots) == 0 {
		return nil, "", fmt.Errorf("no bots available")
	}

	var clients []*PooledClient
	for _, bot := range bots {
		key := fmt.Sprintf("user:%d:bot:%s", userID, bot)
		newPC := &PooledClient{
			Key:      key,
			LastUsed: time.Now(),
		}
		actual, loaded := p.clients.LoadOrStore(key, newPC)
		pc := actual.(*PooledClient)
		if !loaded {
			p.logger.Debug("client.created_slot", zap.String("key", key))
		}
		clients = append(clients, pc)
	}

	selected := p.selectClient(clients)
	if selected == nil {
		return nil, "", fmt.Errorf("no clients available")
	}

	// Fast path: client is ready
	if atomic.LoadInt32(&selected.IsReady) == 1 {
		p.logger.Debug("client.reuse", zap.String("key", selected.Key))
		p.Acquire(selected.Key)
		return selected.TgClient, selected.Key, nil
	}

	// Slow path: use atomic CAS to claim creation
	if !atomic.CompareAndSwapInt32(&selected.Creating, 0, 1) {
		// Another goroutine is creating, wait and retry
		for atomic.LoadInt32(&selected.Creating) == 1 {
			time.Sleep(10 * time.Millisecond)
		}
		// Retry from beginning
		return p.GetBotClient(userID, bots)
	}

	// We claimed creation, create the client
	defer atomic.StoreInt32(&selected.Creating, 0)

	// Double check after claiming
	actual, ok := p.clients.Load(selected.Key)
	if !ok {
		return nil, "", fmt.Errorf("client disappeared")
	}
	pc := actual.(*PooledClient)
	if atomic.LoadInt32(&pc.IsReady) == 1 {
		p.Acquire(pc.Key)
		return pc.TgClient, pc.Key, nil
	}

	token := strings.TrimPrefix(selected.Key, fmt.Sprintf("user:%d:bot:", userID))
	err := p.createBotClient(selected.Key, token)
	if err != nil {
		return nil, "", err
	}
	return pc.TgClient, pc.Key, nil
}

func (p *ClientPool) createUserClient(key string, session *models.Session) error {

	p.logger.Debug("client.create", zap.String("key", key), zap.Int64("user_id", session.UserId))

	ctx := context.Background()

	client, err := AuthClient(ctx, p.cnf, session.Session, NewMiddleware(p.cnf,
		WithFloodWait(),
		WithRecovery(ctx),
		WithRetry(5),
		WithRateLimit(),
	)...)
	if err != nil {
		return err
	}
	return p.startClient(ctx, client, key, "")
}

func (p *ClientPool) createBotClient(key string, token string) error {
	tokenPreview := token
	if len(token) > 10 {
		tokenPreview = token[:10] + "..."
	}
	p.logger.Debug("client.create", zap.String("key", key), zap.String("token", tokenPreview))

	ctx := context.Background()

	client, err := BotClient(ctx, p.db, p.cache, p.cnf, token)
	if err != nil {
		return err
	}

	return p.startClient(ctx, client, key, token)
}

func (p *ClientPool) startClient(clientCtx context.Context, client *telegram.Client, key string, token string) error {
	ready, stopFn, err := client.RunBackground(clientCtx)
	if err != nil {
		return fmt.Errorf("failed to start client: %w", err)
	}

	go func() {
		if err := Auth(clientCtx, client, token); err != nil {
			p.logger.Error("client.auth_failed", zap.String("key", key), zap.Error(err))
			stopFn()
		}
	}()

	select {
	case <-ready:
	case <-time.After(30 * time.Second):
		stopFn()
		return fmt.Errorf("timeout waiting for telegram client")
	case <-clientCtx.Done():
		stopFn()
		return fmt.Errorf("client context cancelled")
	}

	actual, _ := p.clients.Load(key)
	pc := actual.(*PooledClient)
	pc.Client = client
	pc.stop = stopFn
	pc.LastUsed = time.Now()

	// Mark as ready BEFORE acquiring - prevents premature connection tracking
	atomic.StoreInt32(&pc.IsReady, 1)

	p.logger.Debug("client.ready", zap.String("key", key))
	p.Acquire(key)

	pool := pool.NewPool(client, int64(8), NewMiddleware(p.cnf,
		WithFloodWait(),
		WithRecovery(clientCtx),
		WithRetry(5),
		WithRateLimit(),
	)...)
	pc.Close = pool.Close
	pc.TgClient = pool.Default(clientCtx)
	return nil
}

func (p *ClientPool) Close() error {
	close(p.closeChan)
	p.wg.Wait()

	var keys []any
	p.clients.Range(func(k, value any) bool {
		keys = append(keys, k)
		return true
	})

	for _, k := range keys {
		actual, _ := p.clients.Load(k)
		if actual != nil {
			pc := actual.(*PooledClient)
			if pc.stop != nil {
				pc.stop()
			}
		}
		p.clients.Delete(k)
	}
	return nil
}

func (p *ClientPool) Len() int {
	count := 0
	p.clients.Range(func(key, value any) bool {
		count++
		return true
	})
	return count
}

func (p *ClientPool) TotalConnections() int64 {
	return atomic.LoadInt64(&p.totalConns)
}

func (p *ClientPool) Stats() PoolStats {
	stats := PoolStats{
		TotalClients: p.Len(),
		TotalConns:   atomic.LoadInt64(&p.totalConns),
		ClientStats:  make(map[string]int64),
		Strategy:     p.strategy,
	}

	p.clients.Range(func(key, value any) bool {
		pc := value.(*PooledClient)
		stats.ClientStats[pc.Key] = atomic.LoadInt64(&pc.Connections)
		return true
	})

	return stats
}

func (p *ClientPool) Strategy() RoutingStrategy {
	return p.strategy
}

func (p *ClientPool) idleChecker() {
	defer p.wg.Done()

	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-p.closeChan:
			return
		case <-ticker.C:
			p.clients.Range(func(key, value any) bool {
				pc := value.(*PooledClient)
				conns := atomic.LoadInt64(&pc.Connections)
				if conns == 0 && time.Since(pc.LastUsed) > p.idleTimeout {
					if pc.stop != nil {
						p.logger.Debug("client.closing",
							zap.String("key", pc.Key),
							zap.Duration("idle", time.Since(pc.LastUsed)))
						pc.stop()
					}
					p.clients.Delete(key)
				}
				return true
			})
		}
	}
}
