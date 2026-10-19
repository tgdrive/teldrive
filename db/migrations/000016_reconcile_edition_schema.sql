-- +goose Up
-- The original release used versions 8-12 for bookmarks, folder sizes,
-- preferences, natural sorting and browser download sessions. Earlier Spanish
-- editions already shipped versions 8-10 for these three additions. Goose
-- records version numbers, so reconcile the edition additions even when those
-- numbers were recorded by the original release. New upstream-derived objects
-- in migrations 11-15 are idempotent and preserve pre-existing account data.
ALTER TYPE /* TEMPLATE: schema */file_status ADD VALUE IF NOT EXISTS 'spam';
ALTER TABLE /* TEMPLATE: schema */bots ADD COLUMN IF NOT EXISTS name TEXT;
CREATE TABLE IF NOT EXISTS /* TEMPLATE: schema */incoming_spam (
    grantee_id BIGINT NOT NULL REFERENCES /* TEMPLATE: schema */users(user_id) ON DELETE CASCADE,
    file_id UUID NOT NULL REFERENCES /* TEMPLATE: schema */files(id) ON DELETE CASCADE,
    reported_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (grantee_id, file_id)
);
CREATE INDEX IF NOT EXISTS incoming_spam_expiry_idx ON /* TEMPLATE: schema */incoming_spam (reported_at);

-- +goose Down
-- Shared objects may predate this reconciliation; do not drop user data.
