package queue

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
)

const QueueUploads = "uploads"

func NewClient(pool *pgxpool.Pool, exec Executor) (*river.Client[pgx.Tx], error) {
	workers := river.NewWorkers()
	river.AddWorker(workers, &syncRunWorker{exec: exec})
	river.AddWorker(workers, &syncTransferWorker{exec: exec})
	river.AddWorker(workers, &syncFinalizeWorker{exec: exec})
	river.AddWorker(workers, &cleanOldEventsWorker{exec: exec})
	river.AddWorker(workers, &cleanStaleUploadsWorker{exec: exec})
	river.AddWorker(workers, &cleanPendingFilesWorker{exec: exec})

	return river.NewClient(riverpgxv5.New(pool), &river.Config{
		Schema: "teldrive",
		Queues: map[string]river.QueueConfig{
			river.QueueDefault: {MaxWorkers: 50},
			QueueUploads:       {MaxWorkers: 4},
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
	exec Executor
}

func (w *syncTransferWorker) Work(ctx context.Context, job *river.Job[SyncTransferJobArgs]) error {
	return w.exec.SyncTransfer(ctx, job.Args)
}

type syncFinalizeWorker struct {
	river.WorkerDefaults[SyncFinalizeJobArgs]
	exec Executor
}

func (w *syncFinalizeWorker) Work(ctx context.Context, job *river.Job[SyncFinalizeJobArgs]) error {
	return w.exec.SyncFinalize(ctx, job.Args)
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
