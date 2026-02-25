package queue

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
)

func NewClient(pool *pgxpool.Pool, exec Executor) (*river.Client[pgx.Tx], error) {
	workers := river.NewWorkers()
	river.AddWorker(workers, &syncRunWorker{exec: exec})
	river.AddWorker(workers, &syncTransferWorker{exec: exec})
	river.AddWorker(workers, &syncFinalizeWorker{exec: exec})
	river.AddWorker(workers, &cleanOldEventsUserWorker{exec: exec})
	river.AddWorker(workers, &cleanStaleUploadsUserWorker{exec: exec})
	river.AddWorker(workers, &cleanPendingFilesUserWorker{exec: exec})

	return river.NewClient(riverpgxv5.New(pool), &river.Config{
		Schema: "teldrive",
		Queues: map[string]river.QueueConfig{
			river.QueueDefault: {MaxWorkers: 50},
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

type cleanOldEventsUserWorker struct {
	river.WorkerDefaults[CleanOldEventsUserArgs]
	exec Executor
}

func (w *cleanOldEventsUserWorker) Work(ctx context.Context, job *river.Job[CleanOldEventsUserArgs]) error {
	return w.exec.CleanOldEventsForUser(ctx, job.Args)
}

type cleanStaleUploadsUserWorker struct {
	river.WorkerDefaults[CleanStaleUploadsUserArgs]
	exec Executor
}

func (w *cleanStaleUploadsUserWorker) Work(ctx context.Context, job *river.Job[CleanStaleUploadsUserArgs]) error {
	return w.exec.CleanStaleUploadsForUser(ctx, job.Args)
}

type cleanPendingFilesUserWorker struct {
	river.WorkerDefaults[CleanPendingFilesUserArgs]
	exec Executor
}

func (w *cleanPendingFilesUserWorker) Work(ctx context.Context, job *river.Job[CleanPendingFilesUserArgs]) error {
	return w.exec.CleanPendingFilesForUser(ctx, job.Args.UserID)
}
