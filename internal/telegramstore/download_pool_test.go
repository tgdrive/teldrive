package telegramstore

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gotd/td/tg"
)

type backgroundRunnerStub struct {
	runs  atomic.Int32
	exits atomic.Int32
}

func (r *backgroundRunnerStub) Run(ctx context.Context, _ int64, operation Operation, fn func(context.Context, *tg.Client) error) error {
	if operation != OperationDownload {
		return ErrInvalidRequest
	}
	r.runs.Add(1)
	defer r.exits.Add(1)
	return fn(ctx, new(tg.Client))
}

func TestDownloadClientPoolReusesClientAfterRequestCancellation(t *testing.T) {
	runner := &backgroundRunnerStub{}
	pool := newTestDownloadClientPool(t, runner, DownloadClientPoolConfig{
		ClientsPerUser: 1, MaxClients: 4, MaxSessions: 4,
		IdleTimeout: time.Minute, AcquireTimeout: time.Second,
	})
	storage := NewGotdStorage(runner, WithDownloadClientPool(pool))

	requestCtx, cancelRequest := context.WithCancel(context.Background())
	first, err := storage.OpenDownloadSession(requestCtx, 42)
	if err != nil {
		t.Fatal(err)
	}
	cancelRequest()
	second, err := storage.OpenDownloadSession(context.Background(), 42)
	if err != nil {
		t.Fatal(err)
	}
	if got := runner.runs.Load(); got != 1 {
		t.Fatalf("runner starts = %d, want 1", got)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("second lease close: %v", err)
	}
	if err := second.Close(); err != nil {
		t.Fatal(err)
	}
	closeDownloadClientPool(t, pool)
	if got := runner.exits.Load(); got != 1 {
		t.Fatalf("runner exits = %d, want 1", got)
	}
}

func TestDownloadClientPoolAppliesSessionBackpressure(t *testing.T) {
	runner := &backgroundRunnerStub{}
	pool := newTestDownloadClientPool(t, runner, DownloadClientPoolConfig{
		ClientsPerUser: 1, MaxClients: 1, MaxSessions: 1,
		IdleTimeout: time.Minute, AcquireTimeout: 20 * time.Millisecond,
	})
	first, err := pool.OpenDownloadSession(context.Background(), 7)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.OpenDownloadSession(context.Background(), 7); !errors.Is(err, ErrDownloadClientPoolBusy) {
		t.Fatalf("second acquire error = %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	second, err := pool.OpenDownloadSession(context.Background(), 7)
	if err != nil {
		t.Fatalf("acquire after release: %v", err)
	}
	_ = second.Close()
	closeDownloadClientPool(t, pool)
}

func TestDownloadClientPoolEvictsIdleClient(t *testing.T) {
	runner := &backgroundRunnerStub{}
	pool := newTestDownloadClientPool(t, runner, DownloadClientPoolConfig{
		ClientsPerUser: 1, MaxClients: 1, MaxSessions: 1,
		IdleTimeout: 20 * time.Millisecond, AcquireTimeout: time.Second,
	})
	first, err := pool.OpenDownloadSession(context.Background(), 9)
	if err != nil {
		t.Fatal(err)
	}
	_ = first.Close()
	deadline := time.Now().Add(time.Second)
	for runner.exits.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if runner.exits.Load() != 1 {
		t.Fatal("idle client was not evicted")
	}
	second, err := pool.OpenDownloadSession(context.Background(), 9)
	if err != nil {
		t.Fatal(err)
	}
	if got := runner.runs.Load(); got != 2 {
		t.Fatalf("runner starts after eviction = %d, want 2", got)
	}
	_ = second.Close()
	closeDownloadClientPool(t, pool)
}

func TestDownloadClientPoolEvictsIdleClientAtGlobalCapacity(t *testing.T) {
	runner := &backgroundRunnerStub{}
	pool := newTestDownloadClientPool(t, runner, DownloadClientPoolConfig{
		ClientsPerUser: 1, MaxClients: 1, MaxSessions: 1,
		IdleTimeout: time.Minute, AcquireTimeout: time.Second,
	})
	first, err := pool.OpenDownloadSession(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	_ = first.Close()
	second, err := pool.OpenDownloadSession(context.Background(), 11)
	if err != nil {
		t.Fatal(err)
	}
	if got := runner.runs.Load(); got != 2 {
		t.Fatalf("runner starts after capacity eviction = %d, want 2", got)
	}
	_ = second.Close()
	closeDownloadClientPool(t, pool)
}

func TestDownloadClientPoolUsesConfiguredConnections(t *testing.T) {
	runner := &recordingPooledRunner{}
	pool := newTestDownloadClientPool(t, runner, DownloadClientPoolConfig{
		ClientsPerUser: 1, MaxClients: 1, MaxSessions: 1, ReadParallel: 8,
		IdleTimeout: time.Minute, AcquireTimeout: time.Second,
	})
	session, err := pool.OpenDownloadSession(context.Background(), 42)
	if err != nil {
		t.Fatal(err)
	}
	if runner.pooledCalls != 1 || runner.regularCalls != 0 || runner.connections != 8 {
		t.Fatalf("runner calls = pooled:%d regular:%d connections:%d", runner.pooledCalls, runner.regularCalls, runner.connections)
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	closeDownloadClientPool(t, pool)
}

func newTestDownloadClientPool(t *testing.T, runner Runner, config DownloadClientPoolConfig) *DownloadClientPool {
	t.Helper()
	pool, err := NewDownloadClientPool(runner, config)
	if err != nil {
		t.Fatal(err)
	}
	return pool
}

func closeDownloadClientPool(t *testing.T, pool *DownloadClientPool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := pool.Close(ctx); err != nil {
		t.Fatal(err)
	}
}
