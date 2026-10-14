package telegramstore

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gotd/td/tg"
)

func TestFactoryRateLimitsEachTelegramAccountSeparately(t *testing.T) {
	t.Parallel()
	factory, err := NewFactory(FactoryConfig{
		AppID: 1, AppHash: "test", RateLimit: true, RateInterval: time.Hour, RateBurst: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	// Each call wraps a new invoker, like a new request-scoped client or pool.
	invoke := func(accountID int64) error {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		return factory.wrapInvoker(accountID, nopInvoker{}).Invoke(ctx, &tg.UploadGetFileRequest{}, &tg.UploadFile{})
	}

	if err := invoke(101); err != nil {
		t.Fatalf("first request for bot 101 = %v", err)
	}
	if err := invoke(101); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("second client for bot 101 = %v, want shared budget exhausted", err)
	}
	if err := invoke(102); err != nil {
		t.Fatalf("bot 102 throttled by bot 101: %v", err)
	}
	for range 2 {
		if err := invoke(0); err != nil {
			t.Fatalf("unbound client shared a budget: %v", err)
		}
	}
}
