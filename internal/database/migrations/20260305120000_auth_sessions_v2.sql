-- +goose Up
-- +goose StatementBegin
TRUNCATE TABLE teldrive.sessions;

ALTER TABLE teldrive.sessions DROP CONSTRAINT IF EXISTS sessions_pkey;

ALTER TABLE teldrive.sessions
  DROP COLUMN IF EXISTS session,
  DROP COLUMN IF EXISTS hash;

ALTER TABLE teldrive.sessions
  ADD COLUMN IF NOT EXISTS id uuid,
  ADD COLUMN IF NOT EXISTS tg_session text,
  ADD COLUMN IF NOT EXISTS refresh_token_hash text,
  ADD COLUMN IF NOT EXISTS updated_at timestamp,
  ADD COLUMN IF NOT EXISTS revoked_at timestamp;

ALTER TABLE teldrive.sessions
  ALTER COLUMN id SET NOT NULL,
  ALTER COLUMN tg_session SET NOT NULL,
  ALTER COLUMN created_at SET DEFAULT timezone('utc'::text, now()),
  ALTER COLUMN created_at SET NOT NULL,
  ALTER COLUMN updated_at SET DEFAULT timezone('utc'::text, now()),
  ALTER COLUMN updated_at SET NOT NULL;

ALTER TABLE teldrive.sessions
  ADD CONSTRAINT sessions_pkey PRIMARY KEY (id);

CREATE UNIQUE INDEX IF NOT EXISTS sessions_refresh_token_hash_unq
  ON teldrive.sessions (refresh_token_hash)
  WHERE refresh_token_hash IS NOT NULL;

CREATE INDEX IF NOT EXISTS sessions_user_created_idx
  ON teldrive.sessions (user_id, created_at DESC);

-- +goose StatementEnd
