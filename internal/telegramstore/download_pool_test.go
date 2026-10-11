package telegramstore

import (
	"context"
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

func TestDownloadClientPoolReusesReleasedClientAfterRequestCancellation(t *testing.T) {
	runner := &backgroundRunnerStub{}
	pool := newTestDownloadClientPool(t, runner, DownloadClientPoolConfig{})
	storage := NewGotdStorage(runner, nil, WithDownloadClientPool(pool))

	requestCtx, cancelRequest := context.WithCancel(context.Background())
	first, err := storage.OpenDownloadSession(requestCtx, 42)
	if err != nil {
		t.Fatal(err)
	}
	cancelRequest()
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	second, err := storage.OpenDownloadSession(context.Background(), 42)
	if err != nil {
		t.Fatal(err)
	}
	if got := runner.runs.Load(); got != 1 {
		t.Fatalf("runner starts = %d, want 1", got)
	}
	if err := second.Close(); err != nil {
		t.Fatal(err)
	}
	closeDownloadClientPool(t, pool)
	if got := runner.exits.Load(); got != 1 {
		t.Fatalf("runner exits = %d, want 1", got)
	}
}

func TestDownloadClientPoolSharesBackgroundClientAcrossConcurrentSessions(t *testing.T) {
	runner := &backgroundRunnerStub{}
	pool := newTestDownloadClientPool(t, runner, DownloadClientPoolConfig{Clients: 1})

	first, err := pool.OpenDownloadSession(context.Background(), 7)
	if err != nil {
		t.Fatal(err)
	}
	second, err := pool.OpenDownloadSession(context.Background(), 7)
	if err != nil {
		t.Fatal(err)
	}
	if got := runner.runs.Load(); got != 1 {
		t.Fatalf("runner starts = %d, want 1 shared background client", got)
	}
	firstClient, err := first.(*gotdDownloadSession).client()
	if err != nil {
		t.Fatal(err)
	}
	secondClient, err := second.(*gotdDownloadSession).client()
	if err != nil {
		t.Fatal(err)
	}
	if firstClient != secondClient {
		t.Fatal("concurrent sessions do not share the background Telegram client")
	}
	_ = first.Close()
	_ = second.Close()
	closeDownloadClientPool(t, pool)
}

func TestDownloadClientPoolRotatesAcrossBackgroundClients(t *testing.T) {
	runner := &backgroundRunnerStub{}
	pool := newTestDownloadClientPool(t, runner, DownloadClientPoolConfig{Clients: 2})

	first, err := pool.OpenDownloadSession(context.Background(), 9)
	if err != nil {
		t.Fatal(err)
	}
	second, err := pool.OpenDownloadSession(context.Background(), 9)
	if err != nil {
		t.Fatal(err)
	}
	third, err := pool.OpenDownloadSession(context.Background(), 9)
	if err != nil {
		t.Fatal(err)
	}
	if got := runner.runs.Load(); got != 2 {
		t.Fatalf("runner starts = %d, want 2 background clients", got)
	}
	firstClient, _ := first.(*gotdDownloadSession).client()
	secondClient, _ := second.(*gotdDownloadSession).client()
	thirdClient, _ := third.(*gotdDownloadSession).client()
	if firstClient == secondClient {
		t.Fatal("adjacent sessions were not rotated across background clients")
	}
	if thirdClient != firstClient {
		t.Fatal("rotation did not wrap back to the first background client")
	}
	_ = first.Close()
	_ = second.Close()
	_ = third.Close()
	closeDownloadClientPool(t, pool)
}

func TestDownloadClientPoolReapsIdleClient(t *testing.T) {
	runner := &backgroundRunnerStub{}
	pool := newTestDownloadClientPool(t, runner, DownloadClientPoolConfig{})
	first, err := pool.OpenDownloadSession(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	_ = first.Close()

	pool.mu.Lock()
	entry := pool.entries[10][0]
	entry.lastUsed = time.Now().Add(-defaultDownloadClientIdleTimeout)
	pool.mu.Unlock()
	pool.reapIdle(time.Now())

	deadline := time.Now().Add(time.Second)
	for runner.exits.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if runner.exits.Load() != 1 {
		t.Fatal("idle client was not reaped")
	}
	closeDownloadClientPool(t, pool)
}

func TestDownloadClientPoolRestartsOnlySelectedStoppedSlot(t *testing.T) {
	runner := &backgroundRunnerStub{}
	pool := newTestDownloadClientPool(t, runner, DownloadClientPoolConfig{Clients: 2})

	first, err := pool.OpenDownloadSession(context.Background(), 11)
	if err != nil {
		t.Fatal(err)
	}
	second, err := pool.OpenDownloadSession(context.Background(), 11)
	if err != nil {
		t.Fatal(err)
	}
	secondClient, _ := second.(*gotdDownloadSession).client()
	_ = first.Close()
	_ = second.Close()

	pool.mu.Lock()
	pool.entries[11][0].lastUsed = time.Now().Add(-defaultDownloadClientIdleTimeout)
	pool.mu.Unlock()
	pool.reapIdle(time.Now())

	deadline := time.Now().Add(time.Second)
	for runner.exits.Load() < 1 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if runner.exits.Load() != 1 {
		t.Fatalf("runner exits = %d, want only stopped slot to exit", runner.exits.Load())
	}

	third, err := pool.OpenDownloadSession(context.Background(), 11)
	if err != nil {
		t.Fatal(err)
	}
	if got := runner.runs.Load(); got != 3 {
		t.Fatalf("runner starts = %d, want one lazy slot restart", got)
	}
	_ = third.Close()

	fourth, err := pool.OpenDownloadSession(context.Background(), 11)
	if err != nil {
		t.Fatal(err)
	}
	fourthClient, _ := fourth.(*gotdDownloadSession).client()
	if fourthClient != secondClient {
		t.Fatal("healthy background client was restarted instead of reused")
	}
	if got := runner.runs.Load(); got != 3 {
		t.Fatalf("runner starts = %d, healthy slot should not restart", got)
	}
	_ = fourth.Close()
	closeDownloadClientPool(t, pool)
}

func TestDownloadClientPoolUsesConfiguredConnections(t *testing.T) {
	runner := &recordingPooledRunner{}
	pool := newTestDownloadClientPool(t, runner, DownloadClientPoolConfig{ReadParallel: 8})
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
	pool, err := NewDownloadClientPool(runner, config, nil)
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
