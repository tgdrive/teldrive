-- name: InsertAuditEvent :one
INSERT INTO /* TEMPLATE: schema */audit_events (
    id,
    user_id,
    action,
    resource_type,
    resource_id,
    metadata,
    request_id
) VALUES (
    sqlc.arg(id),
    sqlc.narg(user_id),
    sqlc.arg(action),
    sqlc.arg(resource_type),
    sqlc.narg(resource_id),
    sqlc.narg(metadata),
    sqlc.narg(request_id)
)
RETURNING *;

-- name: ListAuditEvents :many
SELECT *
FROM /* TEMPLATE: schema */audit_events
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
