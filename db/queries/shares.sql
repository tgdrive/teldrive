-- name: CreateFileShare :one
INSERT INTO /* TEMPLATE: schema */file_shares (
    id,
    file_id,
    owner_id,
    token_prefix,
    token_hash,
    password_hash,
    expires_at,
    max_downloads
) VALUES (
    sqlc.arg(id),
    sqlc.arg(file_id),
    sqlc.arg(owner_id),
    sqlc.arg(token_prefix),
    sqlc.arg(token_hash),
    sqlc.narg(password_hash),
    sqlc.narg(expires_at),
    sqlc.narg(max_downloads)
)
RETURNING *;

-- name: ListFileShares :many
SELECT *
FROM /* TEMPLATE: schema */file_shares
WHERE owner_id = sqlc.arg(owner_id)
  AND file_id = sqlc.arg(file_id)
  AND (
    sqlc.narg(after_created_at)::timestamptz IS NULL
    OR (created_at, id) < (
      sqlc.narg(after_created_at)::timestamptz,
      sqlc.narg(after_id)::uuid
    )
  )
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(page_size);

-- name: GetFileShareForOwner :one
SELECT *
FROM /* TEMPLATE: schema */file_shares
WHERE id = sqlc.arg(id)
  AND owner_id = sqlc.arg(owner_id);

-- name: UpdateFileShare :one
UPDATE /* TEMPLATE: schema */file_shares
SET password_hash = CASE
      WHEN sqlc.arg(clear_password)::boolean THEN NULL
      ELSE COALESCE(sqlc.narg(password_hash), password_hash)
    END,
    expires_at = CASE
      WHEN sqlc.arg(clear_expires_at)::boolean THEN NULL
      ELSE COALESCE(sqlc.narg(expires_at), expires_at)
    END,
    max_downloads = CASE
      WHEN sqlc.arg(clear_max_downloads)::boolean THEN NULL
      ELSE COALESCE(sqlc.narg(max_downloads), max_downloads)
    END
WHERE id = sqlc.arg(id)
  AND owner_id = sqlc.arg(owner_id)
  AND revoked_at IS NULL
  AND (
    sqlc.arg(clear_max_downloads)::boolean
    OR sqlc.narg(max_downloads)::bigint IS NULL
    OR sqlc.narg(max_downloads)::bigint >= download_count
  )
RETURNING *;

-- name: GetActiveShareByTokenHash :one
SELECT fs.*, f.name AS file_name, f.kind AS file_kind, f.status AS file_status
FROM /* TEMPLATE: schema */file_shares fs
JOIN /* TEMPLATE: schema */files f ON f.id = fs.file_id
WHERE fs.token_hash = sqlc.arg(token_hash)
  AND fs.revoked_at IS NULL
  AND (fs.expires_at IS NULL OR fs.expires_at > now())
  AND (fs.max_downloads IS NULL OR fs.download_count < fs.max_downloads)
  AND f.status = 'active';

-- name: IncrementShareDownloadCount :one
UPDATE /* TEMPLATE: schema */file_shares
SET download_count = download_count + 1
WHERE id = sqlc.arg(id)
  AND revoked_at IS NULL
  AND (expires_at IS NULL OR expires_at > now())
  AND (max_downloads IS NULL OR download_count < max_downloads)
RETURNING *;

-- name: RevokeFileShare :execrows
UPDATE /* TEMPLATE: schema */file_shares
SET revoked_at = now()
WHERE id = sqlc.arg(id)
  AND owner_id = sqlc.arg(owner_id)
  AND revoked_at IS NULL;

-- name: RevokeExpiredShares :execrows
UPDATE /* TEMPLATE: schema */file_shares
SET revoked_at = now()
WHERE revoked_at IS NULL
  AND expires_at IS NOT NULL
  AND expires_at <= now();
