package telegramstore

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gotd/td/bin"
	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
)

func TestFactorySerializesAuthorizationTransfersAcrossClients(t *testing.T) {
	factory, err := NewFactory(FactoryConfig{AppID: 1, AppHash: "test"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var active atomic.Int32
	var completed atomic.Int32
	var wg sync.WaitGroup
	start := make(chan struct{})
	for range 32 {
		// Each client gets its own hook, but they must share the account lock.
		handler := factory.authTransferHandler(42)
		wg.Go(func() {
			<-start
			if err := handler(ctx, nil, func(gotCtx context.Context) error {
				if gotCtx != ctx {
					t.Error("transfer context lost")
				}
				if active.Add(1) != 1 {
					t.Error("overlapping authorization export/import pairs")
				}
				defer active.Add(-1)
				// Model a network round trip between export and import.
				time.Sleep(time.Millisecond)
				completed.Add(1)
				return nil
			}); err != nil {
				t.Errorf("transfer: %v", err)
			}
		})
	}
	close(start)
	wg.Wait()
	if completed.Load() != 32 {
		t.Fatalf("completed %d transfers, want 32", completed.Load())
	}
}

func TestFactoryAuthorizationTransferIsolationCancellationAndFailure(t *testing.T) {
	factory, err := NewFactory(FactoryConfig{AppID: 1, AppHash: "test"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	started, release := make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)
	authErr := tgerr.New(400, "AUTH_BYTES_INVALID")
	handler := factory.authTransferHandler(42)
	go func() {
		done <- handler(ctx, nil, func(context.Context) error {
			close(started)
			select {
			case <-release:
				return authErr
			case <-ctx.Done():
				return ctx.Err()
			}
		})
	}()
	<-started
	// Another account is not blocked by the transfer already in flight.
	if err := factory.authTransferHandler(43)(ctx, nil, func(context.Context) error { return nil }); err != nil {
		t.Errorf("independent account transfer: %v", err)
	}
	waitCtx, stopWaiting := context.WithTimeout(ctx, 10*time.Millisecond)
	defer stopWaiting()
	if err := handler(waitCtx, nil, func(context.Context) error {
		t.Error("cancelled waiter ran a transfer")
		return nil
	}); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("waiter: %v", err)
	}
	close(release)
	if err := <-done; !errors.Is(err, authErr) {
		t.Errorf("transfer error lost: %v", err)
	}
	// Failure must release the lock so a later download can try again.
	if err := handler(ctx, nil, func(context.Context) error { return nil }); err != nil {
		t.Fatalf("transfer after failure: %v", err)
	}
}

func TestFactoryUnboundClientTransferUsesActualAccount(t *testing.T) {
	factory, err := NewFactory(FactoryConfig{AppID: 1, AppHash: "test"})
	if err != nil {
		t.Fatal(err)
	}
	selfErr := error(nil)
	factory.middleware = []telegram.Middleware{telegram.MiddlewareFunc(func(tg.Invoker) telegram.InvokeFunc {
		return func(_ context.Context, input bin.Encoder, output bin.Decoder) error {
			if _, ok := input.(*tg.UsersGetUsersRequest); !ok {
				t.Fatalf("unexpected request %T", input)
			}
			if selfErr != nil {
				return selfErr
			}
			output.(*tg.UserClassVector).Elems = []tg.UserClass{&tg.User{ID: 42, Bot: true}}
			return nil
		}
	})}
	client, err := factory.New(new(session.StorageMemory))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	known := factory.authTransferHandler(42)
	unbound := factory.authTransferHandler(0)
	// While the known account holds its lock, an unbound client that resolves
	// to the same account must wait on that lock too.
	if err := known(ctx, nil, func(context.Context) error {
		waitCtx, stop := context.WithTimeout(ctx, 10*time.Millisecond)
		defer stop()
		if err := unbound(waitCtx, client, func(context.Context) error {
			t.Error("unbound client bypassed the account lock")
			return nil
		}); !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("unbound waiter: %v", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := unbound(ctx, client, func(context.Context) error { return nil }); err != nil {
		t.Fatalf("unbound transfer after release: %v", err)
	}
	// A client still signing in must retain its unauthorized RPC error, not
	// transfer another account's authorization or swallow the failure.
	selfErr = tgerr.New(401, "AUTH_KEY_UNREGISTERED")
	if err := unbound(ctx, client, func(context.Context) error {
		t.Error("transfer ran without an authenticated identity")
		return nil
	}); !errors.Is(err, selfErr) {
		t.Fatalf("identify account error: %v", err)
	}
}
