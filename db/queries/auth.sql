-- name: UpsertUser :one
INSERT INTO /* TEMPLATE: schema */users (
    user_id,
    display_name,
    username,
    premium
) VALUES (
    sqlc.arg(user_id),
    sqlc.narg(display_name),
    sqlc.narg(username),
    sqlc.arg(premium)
)
ON CONFLICT (user_id) DO UPDATE
SET display_name = EXCLUDED.display_name,
    username = EXCLUDED.username,
    premium = EXCLUDED.premium,
    updated_at = now()
RETURNING *;

-- name: GetUser :one
SELECT *
FROM /* TEMPLATE: schema */users
WHERE user_id = sqlc.arg(user_id);

-- name: CreateTelegramLoginFlow :one
INSERT INTO /* TEMPLATE: schema */telegram_login_flows (
    id,
    method,
    phone_number_ciphertext,
    telegram_state_ciphertext,
    password_required,
    expires_at
) VALUES (
    sqlc.arg(id),
    sqlc.arg(method),
    sqlc.narg(phone_number_ciphertext),
    sqlc.arg(telegram_state_ciphertext),
    sqlc.arg(password_required),
    sqlc.arg(expires_at)
)
RETURNING *;

-- name: GetTelegramLoginFlowForUpdate :one
SELECT *
FROM /* TEMPLATE: schema */telegram_login_flows
WHERE id = sqlc.arg(id)
  AND completed_at IS NULL
  AND expires_at > now()
FOR UPDATE;

-- name: UpdateTelegramLoginFlowState :one
UPDATE /* TEMPLATE: schema */telegram_login_flows
SET telegram_state_ciphertext = sqlc.arg(telegram_state_ciphertext),
    password_required = sqlc.arg(password_required)
WHERE id = sqlc.arg(id)
  AND completed_at IS NULL
  AND expires_at > now()
RETURNING *;

-- name: CompleteTelegramLoginFlow :one
UPDATE /* TEMPLATE: schema */telegram_login_flows
SET completed_at = now()
WHERE id = sqlc.arg(id)
  AND completed_at IS NULL
RETURNING *;

-- name: DeleteExpiredTelegramLoginFlows :execrows
DELETE FROM /* TEMPLATE: schema */telegram_login_flows
WHERE expires_at <= now();

-- name: CreateSession :one
INSERT INTO /* TEMPLATE: schema */sessions (
    id,
    user_id,
    telegram_session,
    refresh_token_hash,
    expires_at
) VALUES (
    sqlc.arg(id),
    sqlc.arg(user_id),
    sqlc.arg(telegram_session),
    sqlc.arg(refresh_token_hash),
    sqlc.arg(expires_at)
)
RETURNING *;

-- name: GetSessionByRefreshTokenHash :one
SELECT *
FROM /* TEMPLATE: schema */sessions
WHERE refresh_token_hash = sqlc.arg(refresh_token_hash)
  AND revoked_at IS NULL
  AND expires_at > now();

-- name: RotateSessionRefreshToken :one
UPDATE /* TEMPLATE: schema */sessions
SET refresh_token_hash = sqlc.arg(new_refresh_token_hash),
    last_used_at = now()
WHERE id = sqlc.arg(session_id)
  AND refresh_token_hash = sqlc.arg(old_refresh_token_hash)
  AND revoked_at IS NULL
  AND expires_at > now()
RETURNING *;

-- name: RevokeSession :execrows
UPDATE /* TEMPLATE: schema */sessions
SET revoked_at = COALESCE(revoked_at, now())
WHERE id = sqlc.arg(session_id)
  AND user_id = sqlc.arg(user_id);

-- name: ListSessions :many
SELECT *
FROM /* TEMPLATE: schema */sessions
WHERE user_id = sqlc.arg(user_id)
  AND revoked_at IS NULL
  AND expires_at > now()
  AND (
    sqlc.narg(after_created_at)::timestamptz IS NULL
    OR (created_at, id) < (
      sqlc.narg(after_created_at)::timestamptz,
      sqlc.narg(after_id)::uuid
    )
  )
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(page_size);
-- name: CreateAPIKey :one
INSERT INTO /* TEMPLATE: schema */api_keys (
    id,
    user_id,
    name,
    key_prefix,
    secret_hash,
    expires_at
) VALUES (
    sqlc.arg(id),
    sqlc.arg(user_id),
    sqlc.arg(name),
    sqlc.arg(key_prefix),
    sqlc.arg(secret_hash),
    sqlc.narg(expires_at)
)
RETURNING *;

-- name: ListAPIKeys :many
SELECT *
FROM /* TEMPLATE: schema */api_keys
WHERE user_id = sqlc.arg(user_id)
  AND (
    sqlc.narg(after_created_at)::timestamptz IS NULL
    OR (created_at, id) < (
      sqlc.narg(after_created_at)::timestamptz,
      sqlc.narg(after_id)::uuid
    )
  )
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(page_size);

-- name: GetActiveAPIKeyByHash :one
SELECT *
FROM /* TEMPLATE: schema */api_keys
WHERE secret_hash = sqlc.arg(secret_hash)
  AND revoked_at IS NULL
  AND (expires_at IS NULL OR expires_at > now());

-- name: TouchAPIKey :exec
UPDATE /* TEMPLATE: schema */api_keys
SET last_used_at = now()
WHERE id = sqlc.arg(id);

-- name: RevokeAPIKey :execrows
UPDATE /* TEMPLATE: schema */api_keys
SET revoked_at = now()
WHERE id = sqlc.arg(id)
  AND user_id = sqlc.arg(user_id)
  AND revoked_at IS NULL;

-- name: GetTelegramLoginFlow :one
SELECT *
FROM /* TEMPLATE: schema */telegram_login_flows
WHERE id = sqlc.arg(id)
  AND completed_at IS NULL
  AND expires_at > now();

-- name: GetActiveSession :one
SELECT *
FROM /* TEMPLATE: schema */sessions
WHERE id = sqlc.arg(session_id)
  AND user_id = sqlc.arg(user_id)
  AND revoked_at IS NULL
  AND expires_at > now();

-- name: GetLatestActiveSessionForUser :one
SELECT *
FROM /* TEMPLATE: schema */sessions
WHERE user_id = sqlc.arg(user_id)
  AND revoked_at IS NULL
  AND expires_at > now()
ORDER BY last_used_at DESC NULLS LAST, created_at DESC
LIMIT 1;

-- name: TouchSession :exec
UPDATE /* TEMPLATE: schema */sessions
SET last_used_at = now()
WHERE id = sqlc.arg(session_id)
  AND revoked_at IS NULL;

-- name: UpdateSessionTelegramSession :execrows
UPDATE /* TEMPLATE: schema */sessions
SET telegram_session = sqlc.arg(telegram_session),
    last_used_at = now()
WHERE id = sqlc.arg(session_id)
  AND revoked_at IS NULL
  AND expires_at > now();
