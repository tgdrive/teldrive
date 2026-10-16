-- name: EnsureUserPreferences :exec
INSERT INTO /* TEMPLATE: schema */user_preferences (user_id)
VALUES (sqlc.arg(user_id))
ON CONFLICT (user_id) DO NOTHING;

-- name: GetUserPreferences :one
SELECT *
FROM /* TEMPLATE: schema */user_preferences
WHERE user_id = sqlc.arg(user_id);

-- name: UpdateUserPreferences :one
UPDATE /* TEMPLATE: schema */user_preferences
SET upload_encryption = COALESCE(sqlc.narg(upload_encryption)::boolean, upload_encryption),
    upload_conflict_policy = COALESCE(sqlc.narg(upload_conflict_policy)::/* TEMPLATE: schema */name_conflict_policy, upload_conflict_policy),
    upload_concurrency = COALESCE(sqlc.narg(upload_concurrency)::integer, upload_concurrency),
    upload_part_size = COALESCE(sqlc.narg(upload_part_size)::bigint, upload_part_size),
    file_sort = COALESCE(sqlc.narg(file_sort)::text, file_sort),
    file_order = COALESCE(sqlc.narg(file_order)::text, file_order),
    theme = COALESCE(sqlc.narg(theme)::text, theme),
    locale = COALESCE(sqlc.narg(locale)::text, locale),
    updated_at = now()
WHERE user_id = sqlc.arg(user_id)
RETURNING *;
