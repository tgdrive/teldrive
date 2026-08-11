package jobs

import (
	"errors"
	"testing"
	"time"
)

func TestCleanupSweepMetadata(t *testing.T) {
	t.Parallel()
	args := CleanupSweepArgs{BatchSize: 25}
	if args.Kind() != CleanupSweepKind {
		t.Fatalf("Kind() = %q", args.Kind())
	}
	opts := args.InsertOpts()
	if opts.Queue != CleanupQueue || opts.MaxAttempts != 3 || opts.Priority != 2 {
		t.Fatalf("InsertOpts() = %#v", opts)
	}
	worker := &UploadCleanupWorker{}
	if got := worker.Timeout(nil); got != 10*time.Minute {
		t.Fatalf("Timeout() = %v", got)
	}
}

func TestRuntimeRejectsMissingDependencies(t *testing.T) {
	t.Parallel()
	if _, err := NewRuntime(nil, nil); !errors.Is(err, ErrRuntimeNotConfigured) {
		t.Fatalf("NewRuntime() error = %v", err)
	}
	var runtime *Runtime
	if err := runtime.Start(nil); !errors.Is(err, ErrRuntimeNotConfigured) {
		t.Fatalf("Start() error = %v", err)
	}
	if err := runtime.Stop(nil); !errors.Is(err, ErrRuntimeNotConfigured) {
		t.Fatalf("Stop() error = %v", err)
	}
	if err := runtime.InsertCleanup(nil, 1); !errors.Is(err, ErrRuntimeNotConfigured) {
		t.Fatalf("InsertCleanup() error = %v", err)
	}
}
