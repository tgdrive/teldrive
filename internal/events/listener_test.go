package events

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

type fakeListenerConnector struct {
	mu    sync.Mutex
	conns []listenerConn
	calls int
}

func (c *fakeListenerConnector) Connect(context.Context, string) (listenerConn, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls++
	if len(c.conns) == 0 {
		return nil, errors.New("no fake listener connection")
	}
	conn := c.conns[0]
	c.conns = c.conns[1:]
	return conn, nil
}

type fakeListenerConn struct {
	mu            sync.Mutex
	notifications []*pgconn.Notification
	err           error
	closed        bool
}

func (c *fakeListenerConn) WaitForNotification(ctx context.Context) (*pgconn.Notification, error) {
	c.mu.Lock()
	if len(c.notifications) > 0 {
		notification := c.notifications[0]
		c.notifications = c.notifications[1:]
		c.mu.Unlock()
		return notification, nil
	}
	if c.err != nil {
		err := c.err
		c.err = nil
		c.mu.Unlock()
		return nil, err
	}
	c.mu.Unlock()
	<-ctx.Done()
	return nil, ctx.Err()
}

func (c *fakeListenerConn) Ping(context.Context) error { return nil }
func (c *fakeListenerConn) Close() {
	c.mu.Lock()
	c.closed = true
	c.mu.Unlock()
}

func TestListenerReconnectsAndDelivers(t *testing.T) {
	t.Parallel()

	first := &fakeListenerConn{err: errors.New("connection lost")}
	second := &fakeListenerConn{notifications: []*pgconn.Notification{{
		Channel: notificationChannel,
		Payload: `{"user_id":42}`,
	}}}
	connector := &fakeListenerConnector{conns: []listenerConn{first, second}}
	hub := NewHub(1)
	wake, unsubscribe, err := hub.Subscribe(42)
	if err != nil {
		t.Fatal(err)
	}
	defer unsubscribe()

	listener := newListenerWithConnector(connector, hub, slog.New(slog.NewTextHandler(io.Discard, nil)), listenerConfig{
		ConnectTimeout: time.Second,
		PingInterval:   10 * time.Millisecond,
		ReconnectMin:   time.Millisecond,
		ReconnectMax:   5 * time.Millisecond,
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := listener.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	select {
	case <-wake:
	case <-time.After(time.Second):
		t.Fatal("notification was not delivered after reconnect")
	}

	closeCtx, closeCancel := context.WithTimeout(context.Background(), time.Second)
	defer closeCancel()
	if err := listener.Close(closeCtx); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	connector.mu.Lock()
	calls := connector.calls
	connector.mu.Unlock()
	if calls < 2 {
		t.Fatalf("Connect() calls = %d, want at least 2", calls)
	}
}
