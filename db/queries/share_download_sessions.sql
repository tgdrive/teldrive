-- name: CreateShareDownloadSession :exec
WITH expired AS (
    DELETE FROM /* TEMPLATE: schema */share_download_sessions
    WHERE expires_at <= now()
)
INSERT INTO /* TEMPLATE: schema */share_download_sessions (token_hash, share_id, password_hash, expires_at)
VALUES (sqlc.arg(token_hash), sqlc.arg(share_id), sqlc.narg(password_hash), sqlc.arg(expires_at));

-- name: HasShareDownloadSession :one
SELECT EXISTS (
    SELECT 1 FROM /* TEMPLATE: schema */share_download_sessions
    WHERE token_hash = sqlc.arg(token_hash)
      AND share_id = sqlc.arg(share_id)
      AND password_hash IS NOT DISTINCT FROM sqlc.narg(password_hash)::text
      AND expires_at > now()
);
