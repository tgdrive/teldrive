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
SET preferences = sqlc.arg(preferences)::jsonb,
    updated_at = now()
WHERE user_id = sqlc.arg(user_id)
RETURNING *;

-- name: GetUserPreferencesForUpdate :one
SELECT * FROM /* TEMPLATE: schema */user_preferences
WHERE user_id = sqlc.arg(user_id)
FOR UPDATE;
