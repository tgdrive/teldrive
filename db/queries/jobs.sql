-- name: ListDeletionPendingRoots :many
SELECT f.user_id, f.id AS file_id
FROM /* TEMPLATE: schema */files f
WHERE f.status = 'deletion_pending'
  AND (
    f.parent_id IS NULL
    OR NOT EXISTS (
      SELECT 1
      FROM /* TEMPLATE: schema */files parent
      WHERE parent.id = f.parent_id
        AND parent.user_id = f.user_id
        AND parent.status = 'deletion_pending'
    )
  )
ORDER BY f.updated_at, f.id
LIMIT sqlc.arg(batch_size);

-- name: ListTrashedRootsBefore :many
SELECT f.user_id, f.id AS file_id
FROM /* TEMPLATE: schema */files f
WHERE f.status = 'trashed'
  AND f.deleted_at IS NOT NULL
  AND f.deleted_at <= sqlc.arg(deleted_before)
  AND (
    f.parent_id IS NULL
    OR NOT EXISTS (
      SELECT 1
      FROM /* TEMPLATE: schema */files parent
      WHERE parent.id = f.parent_id
        AND parent.user_id = f.user_id
        AND parent.status = 'trashed'
    )
  )
ORDER BY f.deleted_at, f.id
LIMIT sqlc.arg(batch_size);
