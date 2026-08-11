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
	return fn(ctx, nil)
}

func (r *recordingPooledRunner) RunPooled(ctx context.Context, userID int64, operation Operation, connections int, fn func(context.Context, *tg.Client) error) error {
	r.pooledCalls++
	r.connections = connections
	return fn(ctx, nil)
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
