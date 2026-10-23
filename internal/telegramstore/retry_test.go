package telegramstore

import (
	"context"
	"errors"
	"slices"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/gotd/td/bin"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
)

func TestIsTransientTelegramErrorMatchesRPCTimeout(t *testing.T) {
	err := tgerr.New(-503, "Timeout")
	if !isTransientTelegramError(err) {
		t.Fatalf("isTransientTelegramError(%v) = false, want true", err)
	}
}

func TestRetryMiddlewareBrokenPipeRecovery(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		factory, err := NewFactory(FactoryConfig{AppID: 1, AppHash: "test", MaxRetries: 3})
		if err != nil {
			t.Fatal(err)
		}
		started := time.Now()
		input := &tg.UploadSaveBigFilePartRequest{FileID: 42, FilePart: 320, FileTotalParts: 400, Bytes: []byte("part bytes")}
		output := &tg.BoolBox{}
		var attempts []time.Duration
		// PooledAPI uses this same middleware chain around the pool invoker.
		invoke := factory.wrapInvoker(6281510898, telegram.InvokeFunc(func(_ context.Context, gotInput bin.Encoder, gotOutput bin.Decoder) error {
			if gotInput != input || gotOutput != output {
				t.Fatal("retry changed the RPC input or output")
			}
			elapsed := time.Since(started)
			attempts = append(attempts, elapsed)
			if elapsed < 1500*time.Millisecond {
				return errors.New("rpcDoRequest: retryUntilAck: send: write: write intermediate: write tcp 172.18.0.4:39140->91.108.56.134:443: write: broken pipe")
			}
			return nil
		}))
		if err := invoke.Invoke(WithClientID(context.Background(), 6281510898), input, output); err != nil {
			t.Fatalf("invoke() error = %v", err)
		}
		want := []time.Duration{0, 500 * time.Millisecond, 1500 * time.Millisecond}
		if !slices.Equal(attempts, want) {
			t.Fatalf("attempt times = %v, want %v", attempts, want)
		}
	})
}

func TestRetryMiddlewareStops(t *testing.T) {
	transient := errors.New("write: broken pipe")
	permanent := tgerr.New(400, "FILE_PART_INVALID")
	for _, tc := range []struct {
		name      string
		max       int
		err       error
		wantCalls int
		wantDelay time.Duration
	}{
		{name: "exhausted", max: 3, err: transient, wantCalls: 4, wantDelay: 3500 * time.Millisecond},
		{name: "disabled", max: 0, err: transient, wantCalls: 1},
		{name: "permanent", max: 3, err: permanent, wantCalls: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				started := time.Now()
				calls := 0
				invoke := (retryMiddleware{max: tc.max}).Handle(telegram.InvokeFunc(func(context.Context, bin.Encoder, bin.Decoder) error {
					calls++
					return tc.err
				}))
				if err := invoke(context.Background(), nil, nil); !errors.Is(err, tc.err) {
					t.Fatalf("invoke() error = %v, want %v", err, tc.err)
				}
				if calls != tc.wantCalls || time.Since(started) != tc.wantDelay {
					t.Fatalf("calls = %d, elapsed = %v; want %d, %v", calls, time.Since(started), tc.wantCalls, tc.wantDelay)
				}
			})
		})
	}
}

func TestRetryMiddlewareCancellation(t *testing.T) {
	for _, timeout := range []time.Duration{0, 100 * time.Millisecond} {
		t.Run(timeout.String(), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), timeout)
				defer cancel()
				started := time.Now()
				calls := 0
				invoke := (retryMiddleware{max: 10}).Handle(telegram.InvokeFunc(func(context.Context, bin.Encoder, bin.Decoder) error {
					calls++
					return errors.New("write: broken pipe")
				}))
				if err := invoke(ctx, nil, nil); !errors.Is(err, context.DeadlineExceeded) {
					t.Fatalf("invoke() error = %v, want deadline exceeded", err)
				}
				wantCalls := 1
				if timeout == 0 {
					wantCalls = 0
				}
				if calls != wantCalls || time.Since(started) != timeout {
					t.Fatalf("calls = %d, elapsed = %v; want %d, %v", calls, time.Since(started), wantCalls, timeout)
				}
			})
		})
	}
}

func TestTelegramRetryDelay(t *testing.T) {
	for attempt, want := range []time.Duration{
		500 * time.Millisecond, time.Second, 2 * time.Second, 4 * time.Second,
		8 * time.Second, 10 * time.Second, 10 * time.Second,
	} {
		if got := telegramRetryDelay(attempt); got != want {
			t.Fatalf("telegramRetryDelay(%d) = %v, want %v", attempt, got, want)
		}
	}
}

func TestRetryMiddlewareRetriesRPCTimeout(t *testing.T) {
	var calls atomic.Int32
	middleware := retryMiddleware{max: 2}
	invoke := middleware.Handle(telegram.InvokeFunc(func(_ context.Context, _ bin.Encoder, _ bin.Decoder) error {
		if calls.Add(1) == 1 {
			return tgerr.New(-503, "Timeout")
		}
		return nil
	}))

	if err := invoke(context.Background(), nil, nil); err != nil {
		t.Fatalf("invoke() error = %v", err)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("calls = %d, want 2", got)
	}
}

func TestRetryMiddlewareRetriesConnectionResetByPeer(t *testing.T) {
	var calls atomic.Int32
	middleware := retryMiddleware{max: 2}
	invoke := middleware.Handle(telegram.InvokeFunc(func(_ context.Context, _ bin.Encoder, _ bin.Decoder) error {
		if calls.Add(1) == 1 {
			return errors.New("send: write: write padded intermediate: write tcp 10.89.0.13:33760->10.89.0.3:443: write: connection reset by peer")
		}
		return nil
	}))

	if err := invoke(context.Background(), nil, nil); err != nil {
		t.Fatalf("invoke() error = %v", err)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("calls = %d, want 2", got)
	}
}
