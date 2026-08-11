package health

import (
	"context"
	"errors"
	"testing"
)

type pingerFunc func(context.Context) error

func (f pingerFunc) Ping(ctx context.Context) error { return f(ctx) }

func TestServiceHealth(t *testing.T) {
	t.Parallel()

	svc := NewService("v2-test", pingerFunc(func(context.Context) error { return nil }))
	if got := svc.Live(); got.State != "ok" || got.Version != "v2-test" {
		t.Fatalf("Live() = %#v", got)
	}
	if got, err := svc.Ready(context.Background()); err != nil || got.State != "ok" {
		t.Fatalf("Ready() = %#v, %v", got, err)
	}
}

func TestServiceReadyFailures(t *testing.T) {
	t.Parallel()

	if got, err := NewService("v2-test", nil).Ready(context.Background()); err == nil || got.State != "degraded" {
		t.Fatalf("nil database readiness = %#v, %v", got, err)
	}
	boom := errors.New("boom")
	got, err := NewService("v2-test", pingerFunc(func(context.Context) error { return boom })).Ready(context.Background())
	if !errors.Is(err, boom) || got.State != "degraded" {
		t.Fatalf("failed database readiness = %#v, %v", got, err)
	}
}
