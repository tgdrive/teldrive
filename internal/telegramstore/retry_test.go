package telegramstore

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/gotd/td/bin"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tgerr"
)

func TestIsTransientTelegramErrorMatchesRPCTimeout(t *testing.T) {
	err := tgerr.New(-503, "Timeout")
	if !isTransientTelegramError(err) {
		t.Fatalf("isTransientTelegramError(%v) = false, want true", err)
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
