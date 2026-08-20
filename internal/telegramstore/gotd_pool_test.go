package telegramstore

import (
	"context"
	"testing"

	"github.com/gotd/td/tg"
)

type recordingPooledRunner struct {
	regularCalls int
	pooledCalls  int
	connections  int
}

func (r *recordingPooledRunner) Run(ctx context.Context, userID int64, operation Operation, fn func(context.Context, *tg.Client) error) error {
	r.regularCalls++
	return fn(ctx, new(tg.Client))
}

func (r *recordingPooledRunner) RunPooled(ctx context.Context, userID int64, operation Operation, connections int, fn func(context.Context, *tg.Client) error) error {
	r.pooledCalls++
	r.connections = connections
	return fn(ctx, new(tg.Client))
}

func TestGotdStorageRunUploadUsesConnectionPool(t *testing.T) {
	t.Parallel()
	runner := &recordingPooledRunner{}
	storage := NewGotdStorage(runner)

	if err := storage.runUpload(context.Background(), 42, 8, func(context.Context, *tg.Client) error { return nil }); err != nil {
		t.Fatalf("runUpload() error = %v", err)
	}
	if runner.pooledCalls != 1 || runner.regularCalls != 0 || runner.connections != 8 {
		t.Fatalf("runner calls = pooled:%d regular:%d connections:%d", runner.pooledCalls, runner.regularCalls, runner.connections)
	}
}

func TestGotdStorageRunUploadUsesRegularRunnerForSingleThread(t *testing.T) {
	t.Parallel()
	runner := &recordingPooledRunner{}
	storage := NewGotdStorage(runner)

	if err := storage.runUpload(context.Background(), 42, 1, func(context.Context, *tg.Client) error { return nil }); err != nil {
		t.Fatalf("runUpload() error = %v", err)
	}
	if runner.regularCalls != 1 || runner.pooledCalls != 0 {
		t.Fatalf("runner calls = pooled:%d regular:%d", runner.pooledCalls, runner.regularCalls)
	}
}

func TestGotdDownloadSessionUsesConnectionPool(t *testing.T) {
	t.Parallel()
	runner := &recordingPooledRunner{}
	storage := NewGotdStorage(runner, WithDownloadReadParallel(8))

	session, err := storage.OpenDownloadSession(context.Background(), 42)
	if err != nil {
		t.Fatalf("OpenDownloadSession() error = %v", err)
	}
	if err := session.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if runner.pooledCalls != 1 || runner.regularCalls != 0 || runner.connections != 8 {
		t.Fatalf("runner calls = pooled:%d regular:%d connections:%d", runner.pooledCalls, runner.regularCalls, runner.connections)
	}
}

func TestRunWithConnectionsFallsBackToRegularRunner(t *testing.T) {
	t.Parallel()
	runner := &sessionCountingRunner{}
	if err := runWithConnections(context.Background(), runner, 42, OperationDownload, 8, func(context.Context, *tg.Client) error { return nil }); err != nil {
		t.Fatalf("runWithConnections() error = %v", err)
	}
	if got := runner.runs.Load(); got != 1 {
		t.Fatalf("runner calls = %d, want 1", got)
	}
}
