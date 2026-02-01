package tgc

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/gotd/td/telegram"
	"github.com/tgdrive/teldrive/internal/cache"
	"github.com/tgdrive/teldrive/internal/config"
	"github.com/tgdrive/teldrive/internal/logging"
	"github.com/tgdrive/teldrive/pkg/models"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type StreamClient struct {
	Client *telegram.Client
	ctx    context.Context
	cancel context.CancelFunc
}

type ClientPool struct {
	mu      sync.Mutex
	clients map[string]*StreamClient
	locks   sync.Map // map[string]*sync.Mutex
	db      *gorm.DB
	cache   cache.Cacher
	cnf     *config.TGConfig
	logger  *zap.Logger
}

func NewClientPool(db *gorm.DB, cache cache.Cacher, cnf *config.TGConfig) *ClientPool {
	return &ClientPool{
		clients: make(map[string]*StreamClient),
		db:      db,
		cache:   cache,
		cnf:     cnf,
		logger:  logging.Component("TG"),
	}
}

func (p *ClientPool) getCreationLock(key string) *sync.Mutex {
	lock, _ := p.locks.LoadOrStore(key, &sync.Mutex{})
	return lock.(*sync.Mutex)
}

func (p *ClientPool) GetClient(ctx context.Context, userID int64, token string) (*telegram.Client, error) {
	key := fmt.Sprintf("%d:%s", userID, token)

	// 1. Fast path: check if client exists and is healthy
	p.mu.Lock()
	sc, ok := p.clients[key]
	p.mu.Unlock()

	if ok {
		select {
		case <-sc.ctx.Done():
			// Client expired, will recreate
			p.mu.Lock()
			delete(p.clients, key)
			p.mu.Unlock()
		default:
			p.logger.Debug("client.reuse", zap.String("key", key))
			return sc.Client, nil
		}
	}

	// 2. Slow path: synchronize creation PER KEY
	lock := p.getCreationLock(key)
	lock.Lock()
	defer lock.Unlock()

	// Double check map after acquiring lock
	p.mu.Lock()
	sc, ok = p.clients[key]
	if ok {
		select {
		case <-sc.ctx.Done():
			delete(p.clients, key)
		default:
			p.mu.Unlock()
			p.logger.Debug("client.reuse_after_lock", zap.String("key", key))
			return sc.Client, nil
		}
	}
	p.mu.Unlock()

	// Create new client
	p.logger.Info("client.create", zap.String("key", key), zap.Int64("user_id", userID), zap.Bool("is_bot", token != ""))

	var (
		client *telegram.Client
		err    error
	)

	clientCtx, cancel := context.WithCancel(context.Background())
	clientCtx = logging.WithContext(clientCtx, p.logger)

	if token == "" {
		sess, err := cache.Fetch(ctx, p.cache, fmt.Sprintf("users:session:%d", userID), 0, func() (*models.Session, error) {
			var s models.Session
			if err := p.db.Where("user_id = ?", userID).First(&s).Error; err != nil {
				return nil, err
			}
			return &s, nil
		})
		if err != nil {
			p.logger.Error("client.session_fetch_failed", zap.String("key", key), zap.Error(err))
			cancel()
			return nil, err
		}

		client, err = AuthClient(clientCtx, p.cnf, sess.Session, NewMiddleware(p.cnf,
			WithFloodWait(),
			WithRecovery(clientCtx),
			WithRetry(5),
			WithRateLimit(),
		)...)
		if err != nil {
			p.logger.Error("client.auth_client_failed", zap.String("key", key), zap.Error(err))
			cancel()
			return nil, err
		}
	} else {
		client, err = BotClient(clientCtx, p.db, p.cache, p.cnf, token, NewMiddleware(p.cnf,
			WithFloodWait(),
			WithRecovery(clientCtx),
			WithRetry(5),
			WithRateLimit(),
		)...)
		if err != nil {
			p.logger.Error("client.bot_client_failed", zap.String("key", key), zap.Error(err))
			cancel()
			return nil, err
		}
	}

	ready := make(chan error, 1)
	go func() {
		err := client.Run(clientCtx, func(ctx context.Context) error {
			if err := Auth(ctx, client, token); err != nil {
				return err
			}
			close(ready)
			<-ctx.Done()
			return nil
		})
		if err != nil {
			select {
			case <-ready:
			default:
				ready <- err
			}
		}
		p.mu.Lock()
		delete(p.clients, key)
		p.mu.Unlock()
	}()

	select {
	case err := <-ready:
		if err != nil {
			p.logger.Error("client.auth_failed", zap.String("key", key), zap.Error(err))
			cancel()
			return nil, err
		}
	case <-time.After(30 * time.Second):
		p.logger.Error("client.auth_timeout", zap.String("key", key), zap.Duration("timeout", 30*time.Second))
		cancel()
		return nil, fmt.Errorf("timeout waiting for telegram auth")
	}

	p.mu.Lock()
	p.clients[key] = &StreamClient{
		Client: client,
		ctx:    clientCtx,
		cancel: cancel,
	}
	p.mu.Unlock()
	p.logger.Info("client.ready", zap.String("key", key))

	return client, nil
}

func (p *ClientPool) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, sc := range p.clients {
		sc.cancel()
	}
	p.clients = make(map[string]*StreamClient)
	return nil
}
