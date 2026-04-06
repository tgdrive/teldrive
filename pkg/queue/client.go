package queue

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/tgdrive/teldrive/internal/config"
)

const QueueUploads = "uploads"

func NewClient(pool *pgxpool.Pool, exec Executor, cfg config.QueueConfig, jobsCfg config.JobsConfig) (*river.Client[pgx.Tx], error) {
	if jobsCfg.SyncTransfer.Timeout == 0 {
		jobsCfg.SyncTransfer.Timeout = 3 * time.Hour
	}

	workers := river.NewWorkers()
	river.AddWorker(workers, &syncRunWorker{exec: exec})
	river.AddWorker(workers, &syncTransferWorker{exec: exec, timeout: jobsCfg.SyncTransfer.Timeout})
	river.AddWorker(workers, &cleanOldEventsWorker{exec: exec})
	river.AddWorker(workers, &cleanStaleUploadsWorker{exec: exec})
	river.AddWorker(workers, &cleanPendingFilesWorker{exec: exec})
	river.AddWorker(workers, &refreshFolderSizesWorker{exec: exec})

	if cfg.DefaultWorkers <= 0 {
		cfg.DefaultWorkers = 50
	}
	if cfg.UploadWorkers <= 0 {
		cfg.UploadWorkers = 4
	}

	return river.NewClient(riverpgxv5.New(pool), &river.Config{
		Schema: "teldrive",
		Queues: map[string]river.QueueConfig{
			river.QueueDefault: {MaxWorkers: cfg.DefaultWorkers},
			QueueUploads:       {MaxWorkers: cfg.UploadWorkers},
		},
		Workers: workers,
	})
}

type syncRunWorker struct {
	river.WorkerDefaults[SyncRunJobArgs]
	exec Executor
}

func (w *syncRunWorker) Work(ctx context.Context, job *river.Job[SyncRunJobArgs]) error {
	return w.exec.SyncRun(ctx, job.Args, job.ID)
}

type syncTransferWorker struct {
	river.WorkerDefaults[SyncTransferJobArgs]
	exec    Executor
	timeout time.Duration
}

func (w *syncTransferWorker) Timeout(*river.Job[SyncTransferJobArgs]) time.Duration {
	return w.timeout
}

func (w *syncTransferWorker) Work(ctx context.Context, job *river.Job[SyncTransferJobArgs]) error {
	return w.exec.SyncTransfer(ctx, job.Args, job.ID)
}

type cleanOldEventsWorker struct {
	river.WorkerDefaults[CleanOldEventsArgs]
	exec Executor
}

func (w *cleanOldEventsWorker) Work(ctx context.Context, job *river.Job[CleanOldEventsArgs]) error {
	return w.exec.CleanOldEventsForUser(ctx, job.Args)
}

type cleanStaleUploadsWorker struct {
	river.WorkerDefaults[CleanStaleUploadsArgs]
	exec Executor
}

func (w *cleanStaleUploadsWorker) Work(ctx context.Context, job *river.Job[CleanStaleUploadsArgs]) error {
	return w.exec.CleanStaleUploadsForUser(ctx, job.Args)
}

type cleanPendingFilesWorker struct {
	river.WorkerDefaults[CleanPendingFilesArgs]
	exec Executor
}

func (w *cleanPendingFilesWorker) Work(ctx context.Context, job *river.Job[CleanPendingFilesArgs]) error {
	return w.exec.CleanPendingFilesForUser(ctx, job.Args.UserID)
}

type refreshFolderSizesWorker struct {
	river.WorkerDefaults[RefreshFolderSizesArgs]
	exec Executor
}

func (w *refreshFolderSizesWorker) Work(ctx context.Context, job *river.Job[RefreshFolderSizesArgs]) error {
	return w.exec.RefreshFolderSizesForUser(ctx, job.Args.UserID)
}
