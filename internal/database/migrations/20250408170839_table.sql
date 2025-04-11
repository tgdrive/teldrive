-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS teldrive.events (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    type text NOT NULL,
    user_id bigint NOT NULL,
    source jsonb,
    created_at timestamp DEFAULT timezone('utc'::text, now()) NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_events_created_at ON teldrive.events (created_at DESC);
-- +goose StatementEnd