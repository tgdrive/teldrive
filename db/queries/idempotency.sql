-- name: GetIdempotencyKey :one
SELECT *
FROM /* TEMPLATE: schema */idempotency_keys
WHERE user_id = sqlc.arg(user_id)
  AND scope = sqlc.arg(scope)
  AND key = sqlc.arg(key)
  AND expires_at > now();

-- name: ReserveIdempotencyKey :one
INSERT INTO /* TEMPLATE: schema */idempotency_keys (
    user_id,
    scope,
    key,
    request_hash,
    expires_at
) VALUES (
    sqlc.arg(user_id),
    sqlc.arg(scope),
    sqlc.arg(key),
    sqlc.arg(request_hash),
    sqlc.arg(expires_at)
)
ON CONFLICT (user_id, scope, key) DO NOTHING
RETURNING *;

-- name: CompleteIdempotencyKey :one
UPDATE /* TEMPLATE: schema */idempotency_keys
SET resource_type = sqlc.narg(resource_type),
    resource_id = sqlc.narg(resource_id),
    response_ciphertext = sqlc.narg(response_ciphertext),
    completed_at = now()
WHERE user_id = sqlc.arg(user_id)
  AND scope = sqlc.arg(scope)
  AND key = sqlc.arg(key)
  AND request_hash = sqlc.arg(request_hash)
RETURNING *;

-- name: DeleteExpiredIdempotencyKeys :execrows
DELETE FROM /* TEMPLATE: schema */idempotency_keys
WHERE expires_at <= now();
