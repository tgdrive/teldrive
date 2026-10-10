-- name: MarkIncomingSpam :execrows
INSERT INTO /* TEMPLATE: schema */incoming_spam (grantee_id, file_id)
SELECT sqlc.arg(grantee_id), f.id
FROM /* TEMPLATE: schema */files f
WHERE f.id = sqlc.arg(file_id) AND f.status = 'active'
  AND f.user_id <> sqlc.arg(grantee_id)
  AND EXISTS (
    SELECT 1 FROM /* TEMPLATE: schema */file_access_grants g
    WHERE g.file_id = f.id AND g.grantee_id = sqlc.arg(grantee_id)
      AND g.revoked_at IS NULL AND (g.expires_at IS NULL OR g.expires_at > now())
  )
ON CONFLICT (grantee_id, file_id) DO NOTHING;

-- name: ListIncomingSpam :many
SELECT f.*, spam.reported_at
FROM /* TEMPLATE: schema */incoming_spam spam
JOIN /* TEMPLATE: schema */files f ON f.id = spam.file_id
WHERE spam.grantee_id = sqlc.arg(grantee_id)
  AND f.status = 'active'
ORDER BY spam.reported_at DESC, f.id;

-- name: RestoreIncomingSpam :execrows
DELETE FROM /* TEMPLATE: schema */incoming_spam AS spam
WHERE spam.grantee_id = sqlc.arg(grantee_id) AND spam.file_id = sqlc.arg(file_id);

-- name: DismissIncomingSpam :exec
WITH revoked AS (
  UPDATE /* TEMPLATE: schema */file_access_grants AS grant_entry
  SET revoked_at = now(), updated_at = now()
  WHERE grant_entry.grantee_id = sqlc.arg(grantee_id) AND grant_entry.file_id = sqlc.arg(file_id)
    AND EXISTS (SELECT 1 FROM /* TEMPLATE: schema */incoming_spam AS spam WHERE spam.grantee_id = sqlc.arg(grantee_id) AND spam.file_id = sqlc.arg(file_id))
  RETURNING id
)
DELETE FROM /* TEMPLATE: schema */incoming_spam AS spam
WHERE spam.grantee_id = sqlc.arg(grantee_id) AND spam.file_id = sqlc.arg(file_id);

-- name: ExpireIncomingSpam :exec
WITH revoked AS (
  UPDATE /* TEMPLATE: schema */file_access_grants g
  SET revoked_at = COALESCE(g.revoked_at, now()), updated_at = now()
  FROM /* TEMPLATE: schema */incoming_spam spam
  WHERE g.grantee_id = spam.grantee_id AND g.file_id = spam.file_id
    AND spam.reported_at <= sqlc.arg(reported_before)
  RETURNING g.id
)
DELETE FROM /* TEMPLATE: schema */incoming_spam AS spam
WHERE spam.reported_at <= sqlc.arg(reported_before);
