-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS teldrive.api_keys (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id bigint NOT NULL REFERENCES teldrive.users(user_id) ON DELETE CASCADE,
  name text NOT NULL,
  token_hash text NOT NULL,
  expires_at timestamp,
  last_used_at timestamp,
  created_at timestamp DEFAULT timezone('utc'::text, now()) NOT NULL,
  updated_at timestamp DEFAULT timezone('utc'::text, now()) NOT NULL,
  revoked_at timestamp
);

CREATE UNIQUE INDEX IF NOT EXISTS api_keys_token_hash_unq
  ON teldrive.api_keys (token_hash);

CREATE INDEX IF NOT EXISTS api_keys_user_created_idx
  ON teldrive.api_keys (user_id, created_at DESC)
  WHERE revoked_at IS NULL;
-- +goose StatementEnd
