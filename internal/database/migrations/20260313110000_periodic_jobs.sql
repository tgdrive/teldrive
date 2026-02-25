-- +goose Up
CREATE TABLE IF NOT EXISTS teldrive.periodic_jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id BIGINT NOT NULL,
    name TEXT NOT NULL,
    kind TEXT NOT NULL,
    args JSONB NOT NULL DEFAULT '{}'::jsonb,
    cron_expression TEXT NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    system BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT periodic_jobs_name_unique UNIQUE (user_id, name),
    CONSTRAINT periodic_jobs_cron_check CHECK (btrim(cron_expression) <> '')
);

CREATE INDEX IF NOT EXISTS idx_periodic_jobs_user_id ON teldrive.periodic_jobs (user_id, created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS teldrive.periodic_jobs;
