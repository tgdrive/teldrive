package events

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const notificationChannel = "teldrive_events"

type listenerConfig struct {
	ConnectTimeout time.Duration
	PingInterval   time.Duration
	ReconnectMin   time.Duration
	ReconnectMax   time.Duration
}

type listenerConn interface {
	WaitForNotification(context.Context) (*pgconn.Notification, error)
	Ping(context.Context) error
	Close()
}

type listenerConnector interface {
	Connect(context.Context, string) (listenerConn, error)
}

type pgxConnector struct {
	pool *pgxpool.Pool
}

func (c pgxConnector) Connect(ctx context.Context, channel string) (listenerConn, error) {
	conn, err := c.pool.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := conn.Exec(ctx, "LISTEN "+pgx.Identifier{channel}.Sanitize()); err != nil {
		conn.Release()
		return nil, err
	}
	return &dedicatedListenerConn{conn: conn.Hijack()}, nil
}

type dedicatedListenerConn struct {
	conn *pgx.Conn
}

func (c *dedicatedListenerConn) WaitForNotification(ctx context.Context) (*pgconn.Notification, error) {
	return c.conn.WaitForNotification(ctx)
}

func (c *dedicatedListenerConn) Ping(ctx context.Context) error {
	return c.conn.Ping(ctx)
}

func (c *dedicatedListenerConn) Close() {
	if c == nil || c.conn == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = c.conn.Close(ctx)
	c.conn = nil
}

type notificationPayload struct {
	UserID int64 `json:"user_id"`
}

// Listener owns one dedicated PostgreSQL connection and reconnects after
// transport failures. Notifications only wake local subscribers; PostgreSQL
// rows remain the source of truth.
type Listener struct {
	connector listenerConnector
	hub       *Hub
	logger    *slog.Logger
	config    listenerConfig

	mu      sync.Mutex
	cancel  context.CancelFunc
	done    chan struct{}
	running bool
}

func newListener(pool *pgxpool.Pool, hub *Hub, logger *slog.Logger, cfg listenerConfig) *Listener {
	return newListenerWithConnector(pgxConnector{pool: pool}, hub, logger, cfg)
}

func newListenerWithConnector(connector listenerConnector, hub *Hub, logger *slog.Logger, cfg listenerConfig) *Listener {
	if logger == nil {
		logger = slog.Default()
	}
	return &Listener{connector: connector, hub: hub, logger: logger, config: cfg}
}

func (l *Listener) Start(ctx context.Context) error {
	if l == nil || l.connector == nil || l.hub == nil {
		return errors.New("event listener is not configured")
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	if l.running {
		return nil
	}

	serviceCtx, cancel := context.WithCancel(ctx)
	conn, err := l.connect(serviceCtx)
	if err != nil {
		cancel()
		return fmt.Errorf("connect event listener: %w", err)
	}

	l.cancel = cancel
	l.done = make(chan struct{})
	l.running = true
	go l.run(serviceCtx, conn, l.done)
	return nil
}

func (l *Listener) Close(ctx context.Context) error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	if !l.running {
		l.mu.Unlock()
		return nil
	}
	cancel, done := l.cancel, l.done
	l.mu.Unlock()

	cancel()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (l *Listener) run(ctx context.Context, conn listenerConn, done chan struct{}) {
	defer func() {
		if conn != nil {
			conn.Close()
		}
		l.mu.Lock()
		l.running = false
		l.cancel = nil
		close(done)
		l.mu.Unlock()
	}()

	attempt := 0
	for {
		err := l.wait(ctx, conn)
		conn.Close()
		conn = nil
		if ctx.Err() != nil {
			return
		}

		delay := jitterDelay(reconnectDelay(l.config.ReconnectMin, l.config.ReconnectMax, attempt))
		attempt++
		l.logger.ErrorContext(ctx, "event listener disconnected; reconnecting", "error", err, "delay", delay)
		if err := sleepContext(ctx, delay); err != nil {
			return
		}

		for {
			var connectErr error
			conn, connectErr = l.connect(ctx)
			if connectErr == nil {
				break
			}
			delay = jitterDelay(reconnectDelay(l.config.ReconnectMin, l.config.ReconnectMax, attempt))
			attempt++
			l.logger.ErrorContext(ctx, "event listener reconnect failed", "error", connectErr, "delay", delay)
			if err := sleepContext(ctx, delay); err != nil {
				return
			}
		}
	}
}

func (l *Listener) connect(ctx context.Context) (listenerConn, error) {
	connectCtx, cancel := context.WithTimeout(ctx, l.config.ConnectTimeout)
	defer cancel()
	return l.connector.Connect(connectCtx, notificationChannel)
}

func (l *Listener) wait(ctx context.Context, conn listenerConn) error {
	for {
		waitCtx, cancel := context.WithTimeout(ctx, l.config.PingInterval)
		notification, err := conn.WaitForNotification(waitCtx)
		cancel()
		if err == nil {
			l.deliver(notification)
			continue
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if errors.Is(err, context.DeadlineExceeded) {
			pingCtx, pingCancel := context.WithTimeout(ctx, l.config.ConnectTimeout)
			pingErr := conn.Ping(pingCtx)
			pingCancel()
			if pingErr == nil {
				continue
			}
			return fmt.Errorf("ping event listener: %w", pingErr)
		}
		return fmt.Errorf("wait for event notification: %w", err)
	}
}

func (l *Listener) deliver(notification *pgconn.Notification) {
	if notification == nil || notification.Channel != notificationChannel {
		return
	}
	var payload notificationPayload
	if err := json.Unmarshal([]byte(notification.Payload), &payload); err != nil || payload.UserID <= 0 {
		l.logger.Warn("ignored malformed event notification", "payload", notification.Payload, "error", err)
		return
	}
	l.hub.Notify(payload.UserID)
}

func reconnectDelay(minimum, maximum time.Duration, attempt int) time.Duration {
	if minimum <= 0 {
		minimum = 100 * time.Millisecond
	}
	if maximum < minimum {
		maximum = minimum
	}
	if attempt < 0 {
		attempt = 0
	}
	if attempt > 30 {
		attempt = 30
	}
	multiplier := math.Pow(2, float64(attempt))
	delay := time.Duration(float64(minimum) * multiplier)
	if delay < minimum || delay > maximum {
		return maximum
	}
	return delay
}

func jitterDelay(base time.Duration) time.Duration {
	spread := base / 5
	if spread <= 0 {
		return base
	}
	return base - spread + time.Duration(rand.Int64N(int64(2*spread)+1))
}

func sleepContext(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
