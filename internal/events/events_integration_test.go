//go:build integration

package events_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/tgdrive/teldrive/v2/internal/events"
	testpostgres "github.com/tgdrive/teldrive/v2/internal/testutil/postgres"
)

func TestServiceDurableReplayNotificationsAndTickets(t *testing.T) {
	db := testpostgres.New(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := db.Pool.Exec(ctx, "INSERT INTO users (user_id) VALUES (1001), (1002)"); err != nil {
		t.Fatal(err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := events.Config{
		BatchSize:             10,
		MaxConnectionsPerUser: 5,
		Heartbeat:             20 * time.Millisecond,
		WriteTimeout:          time.Second,
		TicketTTL:             80 * time.Millisecond,
		Retention:             time.Hour,
		CleanupInterval:       time.Hour,
		ConnectTimeout:        time.Second,
		PingInterval:          20 * time.Millisecond,
		ReconnectMin:          time.Millisecond,
		ReconnectMax:          10 * time.Millisecond,
	}
	first, err := events.NewService(db.Pool, logger, cfg)
	if err != nil {
		t.Fatalf("NewService(first) error = %v", err)
	}
	second, err := events.NewService(db.Pool, logger, cfg)
	if err != nil {
		t.Fatalf("NewService(second) error = %v", err)
	}
	if err := first.Start(ctx); err != nil {
		t.Fatalf("first.Start() error = %v", err)
	}
	if err := second.Start(ctx); err != nil {
		t.Fatalf("second.Start() error = %v", err)
	}
	defer closeService(t, first)
	defer closeService(t, second)

	firstWake, unsubscribeFirst, err := first.Subscribe(1001)
	if err != nil {
		t.Fatal(err)
	}
	defer unsubscribeFirst()
	secondWake, unsubscribeSecond, err := second.Subscribe(1001)
	if err != nil {
		t.Fatal(err)
	}
	defer unsubscribeSecond()
	otherWake, unsubscribeOther, err := first.Subscribe(1002)
	if err != nil {
		t.Fatal(err)
	}
	defer unsubscribeOther()

	var fileID string
	if err := db.Pool.QueryRow(ctx, `
		INSERT INTO files (user_id, name, normalized_name, kind, mod_time)
		VALUES (1001, 'first', 'first', 'folder', now())
		RETURNING id::text
	`).Scan(&fileID); err != nil {
		t.Fatalf("insert file: %v", err)
	}
	waitWake(t, firstWake, "first instance")
	waitWake(t, secondWake, "second instance")
	select {
	case <-otherWake:
		t.Fatal("event notification leaked to another user")
	case <-time.After(50 * time.Millisecond):
	}

	replayed, err := first.ListAfter(ctx, 1001, 0, nil)
	if err != nil {
		t.Fatalf("ListAfter() error = %v", err)
	}
	if len(replayed) != 1 || replayed[0].Type != "file.created" || replayed[0].ResourceID != fileID {
		t.Fatalf("replayed events = %#v", replayed)
	}
	cursor := replayed[0].ID

	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO files (user_id, name, normalized_name, kind, mod_time)
		VALUES (1001, 'rolled-back', 'rolled-back', 'folder', now())
	`); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
		t.Fatal(err)
	}
	select {
	case <-firstWake:
		t.Fatal("rolled-back mutation emitted a notification")
	case <-time.After(50 * time.Millisecond):
	}
	rows, err := first.ListAfter(ctx, 1001, cursor, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("rolled-back mutation created events: %#v", rows)
	}

	if _, err := db.Pool.Exec(ctx, "UPDATE files SET name = 'second', normalized_name = 'second', generation = generation + 1 WHERE id = $1", fileID); err != nil {
		t.Fatal(err)
	}
	waitWake(t, firstWake, "file update")
	rows, err = first.ListAfter(ctx, 1001, cursor, []string{"file.updated"})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Type != "file.updated" {
		t.Fatalf("updated events = %#v", rows)
	}
	filtered, err := first.ListAfter(ctx, 1001, cursor, []string{"file.created"})
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered) != 0 {
		t.Fatalf("event type filter returned %#v", filtered)
	}

	if _, err := db.Pool.Exec(ctx, "DELETE FROM user_events WHERE user_id = 1001"); err != nil {
		t.Fatal(err)
	}
	expired, err := first.CursorExpired(ctx, 1001, cursor)
	if err != nil {
		t.Fatal(err)
	}
	if !expired {
		t.Fatal("deleted replay cursor was not reported as expired")
	}
	if _, err := first.CursorExpired(ctx, 1001, rows[0].ID+1000); !errors.Is(err, events.ErrInvalidCursor) {
		t.Fatalf("future cursor error = %v", err)
	}

	ticket, err := first.IssueTicket(ctx, 1001)
	if err != nil {
		t.Fatalf("IssueTicket() error = %v", err)
	}
	if got, err := first.AuthenticateTicket(ctx, ticket.Value); err != nil || got != 1001 {
		t.Fatalf("AuthenticateTicket() = %d, %v", got, err)
	}
	if _, err := first.AuthenticateTicket(ctx, "invalid"); !errors.Is(err, events.ErrInvalidTicket) {
		t.Fatalf("invalid ticket error = %v", err)
	}
	time.Sleep(100 * time.Millisecond)
	if _, err := first.AuthenticateTicket(ctx, ticket.Value); !errors.Is(err, events.ErrInvalidTicket) {
		t.Fatalf("expired ticket error = %v", err)
	}

	closeService(t, first)
	var listeningChannels int
	if err := db.Pool.QueryRow(ctx, "SELECT count(*) FROM pg_listening_channels()").Scan(&listeningChannels); err != nil {
		t.Fatalf("inspect pooled LISTEN state: %v", err)
	}
	if listeningChannels != 0 {
		t.Fatalf("pooled connection retained %d LISTEN subscriptions", listeningChannels)
	}
	if _, _, err := first.Subscribe(1001); !errors.Is(err, events.ErrServiceClosed) {
		t.Fatalf("closed service subscription error = %v", err)
	}
	if err := first.Start(ctx); !errors.Is(err, events.ErrServiceClosed) {
		t.Fatalf("closed service restart error = %v", err)
	}
	if _, err := first.IssueTicket(ctx, 1001); !errors.Is(err, events.ErrServiceClosed) {
		t.Fatalf("closed service ticket error = %v", err)
	}
}

func waitWake(t *testing.T, wake <-chan struct{}, name string) {
	t.Helper()
	select {
	case <-wake:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %s notification", name)
	}
}

func closeService(t *testing.T, service *events.Service) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := service.Close(ctx); err != nil {
		t.Errorf("Close() error = %v", err)
	}
}
