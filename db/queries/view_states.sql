-- name: GetFileViewState :one
SELECT *
FROM /* TEMPLATE: schema */file_view_states
WHERE user_id = sqlc.arg(user_id)
  AND file_id = sqlc.arg(file_id);

-- name: UpsertFileViewState :one
INSERT INTO /* TEMPLATE: schema */file_view_states (
    user_id, file_id, viewer_kind, position, preferences, bookmarks
) SELECT
    sqlc.arg(user_id), sqlc.arg(file_id), sqlc.arg(viewer_kind),
    sqlc.arg(position), sqlc.arg(preferences), sqlc.arg(bookmarks)
FROM /* TEMPLATE: schema */files
WHERE id = sqlc.arg(file_id)
  AND kind = 'file'
  AND status = 'active'
ON CONFLICT (user_id, file_id) DO UPDATE
SET viewer_kind = EXCLUDED.viewer_kind,
    position = EXCLUDED.position,
    preferences = EXCLUDED.preferences,
    bookmarks = EXCLUDED.bookmarks,
    updated_at = now()
RETURNING *;

-- name: DeleteFileViewState :execrows
DELETE FROM /* TEMPLATE: schema */file_view_states
WHERE user_id = sqlc.arg(user_id)
  AND file_id = sqlc.arg(file_id);
