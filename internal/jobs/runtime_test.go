package jobs

import (
	"errors"
	"testing"
	"time"
)

func TestUploadCleanupSweepMetadata(t *testing.T) {
	t.Parallel()
	args := UploadCleanupSweepArgs{BatchSize: 25}
	if args.Kind() != UploadCleanupSweepKind {
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

func TestTrashCleanupPeriodicTemplate(t *testing.T) {
	t.Parallel()
	runtime := &Runtime{purgeEnabled: true}
	for _, template := range runtime.PeriodicJobCatalog() {
		if template.Kind != TrashCleanupSweepKind {
			continue
		}
		if template.ID != trashCleanupPeriodicID || template.DefaultCronExpression != trashCleanupDefaultCron {
			t.Fatalf("trash template = %#v", template)
		}
		if got := string(template.DefaultArgs["retention"]); got != `"720h"` {
			t.Fatalf("trash retention arg = %s", got)
		}
		if got := string(template.DefaultArgs["batch_size"]); got != "100" {
			t.Fatalf("trash batch arg = %s", got)
		}
		return
	}
	t.Fatal("trash cleanup periodic template not found")
}

func TestPendingDeletionCleanupPeriodicTemplate(t *testing.T) {
	t.Parallel()
	runtime := &Runtime{purgeEnabled: true}
	for _, template := range runtime.PeriodicJobCatalog() {
		if template.Kind != PurgeSweepKind {
			continue
		}
		if template.Label != "Pending deletion cleanup" || template.DefaultCronExpression != pendingDeletionCleanupDefaultCron {
			t.Fatalf("pending deletion template = %#v", template)
		}
		if template.DefaultCronExpression != "@every 12h" {
			t.Fatalf("pending deletion schedule = %q", template.DefaultCronExpression)
		}
		return
	}
	t.Fatal("pending deletion cleanup periodic template not found")
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
