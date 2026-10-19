-- +goose Up

-- Browser-only download credentials. Store a digest, never the bearer secret or
-- plaintext password. Binding the session to the current password hash makes a
-- password change invalidate previously authorized browsers immediately.
CREATE TABLE /* TEMPLATE: schema */share_download_sessions (
    token_hash BYTEA PRIMARY KEY,
    share_id UUID NOT NULL REFERENCES /* TEMPLATE: schema */file_shares(id) ON DELETE CASCADE,
    password_hash TEXT,
    expires_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX share_download_sessions_expires_at_idx
    ON /* TEMPLATE: schema */share_download_sessions (expires_at);

-- +goose Down

DROP TABLE IF EXISTS /* TEMPLATE: schema */share_download_sessions;
