-- +goose Up
CREATE TABLE IF NOT EXISTS teldrive.cron_jobs (
    name TEXT PRIMARY KEY,
    schedule_type TEXT NOT NULL CHECK (schedule_type IN ('interval', 'cron')),
    interval_seconds INTEGER,
    cron_expression TEXT,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    next_run_at TIMESTAMPTZ NOT NULL,
    last_run_at TIMESTAMPTZ,
    locked_by TEXT,
    locked_until TIMESTAMPTZ,
    timeout_seconds INTEGER NOT NULL DEFAULT 60,
    max_retries INTEGER NOT NULL DEFAULT 0,
    retry_backoff_seconds INTEGER NOT NULL DEFAULT 30,
    jitter_percentage DOUBLE PRECISION NOT NULL DEFAULT 0,
    attempt INTEGER NOT NULL DEFAULT 0,
    consecutive_failures INTEGER NOT NULL DEFAULT 0,
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT cron_jobs_schedule_check CHECK (
        (schedule_type = 'interval' AND interval_seconds IS NOT NULL AND interval_seconds > 0 AND cron_expression IS NULL) OR
        (schedule_type = 'cron' AND cron_expression IS NOT NULL AND interval_seconds IS NULL)
    )
);

CREATE INDEX IF NOT EXISTS idx_cron_jobs_due ON teldrive.cron_jobs (enabled, next_run_at);
CREATE INDEX IF NOT EXISTS idx_cron_jobs_locked_until ON teldrive.cron_jobs (locked_until);

-- +goose Down
DROP TABLE IF EXISTS teldrive.cron_jobs;
