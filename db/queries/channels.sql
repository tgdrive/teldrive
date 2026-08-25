-- name: ListChannels :many
SELECT *
FROM /* TEMPLATE: schema */channels
WHERE user_id = sqlc.arg(user_id)
  AND (
    sqlc.narg(after_created_at)::timestamptz IS NULL
    OR (created_at, channel_id) < (
      sqlc.narg(after_created_at)::timestamptz,
      sqlc.narg(after_channel_id)::bigint
    )
  )
ORDER BY created_at DESC, channel_id DESC
LIMIT sqlc.arg(page_size);

-- name: GetChannelForUser :one
SELECT *
FROM /* TEMPLATE: schema */channels
WHERE user_id = sqlc.arg(user_id)
  AND channel_id = sqlc.arg(channel_id);

-- name: GetSelectedChannel :one
SELECT *
FROM /* TEMPLATE: schema */channels
WHERE user_id = sqlc.arg(user_id)
  AND selected;

-- name: CreateChannel :one
INSERT INTO /* TEMPLATE: schema */channels (
    channel_id,
    user_id,
    name,
    selected
) VALUES (
    sqlc.arg(channel_id),
    sqlc.arg(user_id),
    sqlc.arg(name),
    FALSE
)
RETURNING *;

-- name: ClearSelectedChannel :exec
UPDATE /* TEMPLATE: schema */channels
SET selected = FALSE,
    updated_at = now()
WHERE user_id = sqlc.arg(user_id)
  AND selected;

-- name: SelectChannel :one
UPDATE /* TEMPLATE: schema */channels
SET selected = TRUE,
    updated_at = now()
WHERE user_id = sqlc.arg(user_id)
  AND channel_id = sqlc.arg(channel_id)
RETURNING *;

-- name: UpdateChannelHealth :one
UPDATE /* TEMPLATE: schema */channels
SET health = sqlc.arg(health),
    last_checked_at = now(),
    updated_at = now()
WHERE user_id = sqlc.arg(user_id)
  AND channel_id = sqlc.arg(channel_id)
RETURNING *;

-- name: DeleteChannel :execrows
DELETE FROM /* TEMPLATE: schema */channels
WHERE user_id = sqlc.arg(user_id)
  AND channel_id = sqlc.arg(channel_id)
  AND NOT selected;
-- name: ListBots :many
SELECT *
FROM /* TEMPLATE: schema */bots
WHERE user_id = sqlc.arg(user_id)
  AND (
    sqlc.narg(after_created_at)::timestamptz IS NULL
    OR (created_at, bot_id) < (
      sqlc.narg(after_created_at)::timestamptz,
      sqlc.narg(after_bot_id)::bigint
    )
  )
ORDER BY created_at DESC, bot_id DESC
LIMIT sqlc.arg(page_size);

-- name: InsertPendingBot :execrows
INSERT INTO /* TEMPLATE: schema */bots (
    bot_id,
    user_id,
    token_ciphertext,
    enabled
) VALUES (
    sqlc.arg(bot_id),
    sqlc.arg(user_id),
    sqlc.arg(token_ciphertext),
    FALSE
)
ON CONFLICT (user_id, bot_id) DO NOTHING;

-- name: GetBot :one
SELECT *
FROM /* TEMPLATE: schema */bots
WHERE user_id = sqlc.arg(user_id)
  AND bot_id = sqlc.arg(bot_id);

-- name: ActivateBot :one
UPDATE /* TEMPLATE: schema */bots
SET username = sqlc.arg(username),
    enabled = TRUE,
    consecutive_failures = 0,
    last_error = NULL,
    retry_after = NULL,
    updated_at = now()
WHERE user_id = sqlc.arg(user_id)
  AND bot_id = sqlc.arg(bot_id)
RETURNING *;

-- name: MarkBotProvisionFailure :execrows
UPDATE /* TEMPLATE: schema */bots
SET enabled = FALSE,
    consecutive_failures = consecutive_failures + 1,
    last_error = sqlc.arg(last_error),
    updated_at = now()
WHERE user_id = sqlc.arg(user_id)
  AND bot_id = sqlc.arg(bot_id);

-- name: DeleteBot :execrows
DELETE FROM /* TEMPLATE: schema */bots
WHERE user_id = sqlc.arg(user_id)
  AND bot_id = sqlc.arg(bot_id);

-- name: ListEnabledBots :many
SELECT *
FROM /* TEMPLATE: schema */bots
WHERE user_id = sqlc.arg(user_id)
  AND enabled
ORDER BY bot_id;

-- name: ListUploadEligibleBots :many
SELECT *
FROM /* TEMPLATE: schema */bots
WHERE user_id = sqlc.arg(user_id)
  AND enabled
  AND (retry_after IS NULL OR retry_after <= now())
ORDER BY bot_id;

-- name: NextBotSelectionValue :one
INSERT INTO /* TEMPLATE: schema */bot_selection_counters (
    user_id,
    operation,
    next_value
) VALUES (
    sqlc.arg(user_id),
    sqlc.arg(operation),
    1
)
ON CONFLICT (user_id, operation) DO UPDATE
SET next_value = /* TEMPLATE: schema */bot_selection_counters.next_value + 1,
    updated_at = now()
RETURNING (next_value - 1)::bigint AS selection_value;
-- name: CountChannelReferences :one
SELECT (
    (SELECT count(*) FROM /* TEMPLATE: schema */file_parts fp WHERE fp.channel_id = sqlc.arg(target_channel_id)) +
    (SELECT count(*) FROM /* TEMPLATE: schema */upload_parts up WHERE up.channel_id = sqlc.arg(target_channel_id))
)::bigint AS reference_count;

-- name: CountChannelStoredMessages :one
SELECT count(*)::bigint
FROM (
    SELECT fp.channel_id, fp.message_id
    FROM /* TEMPLATE: schema */file_parts fp
    WHERE fp.channel_id = sqlc.arg(target_channel_id)
    UNION
    SELECT up.channel_id, up.message_id
    FROM /* TEMPLATE: schema */upload_parts up
    WHERE up.channel_id = sqlc.arg(target_channel_id)
      AND message_id IS NOT NULL
) AS stored_messages;

-- name: ListChannelsForOrphanCleanup :many
SELECT *
FROM /* TEMPLATE: schema */channels
ORDER BY user_id, channel_id;

-- name: ListReferencedMessageIDs :many
SELECT message_id
FROM (
    SELECT fp.message_id
    FROM /* TEMPLATE: schema */file_parts fp
    WHERE fp.channel_id = sqlc.arg(target_channel_id)
      AND fp.message_id = ANY(sqlc.arg(message_ids)::bigint[])
    UNION
    SELECT up.message_id
    FROM /* TEMPLATE: schema */upload_parts up
    WHERE up.channel_id = sqlc.arg(target_channel_id)
      AND up.message_id = ANY(sqlc.arg(message_ids)::bigint[])
) AS referenced_messages;

-- name: UpsertDiscoveredChannel :one
INSERT INTO /* TEMPLATE: schema */channels (
    channel_id,
    user_id,
    name,
    selected,
    health
) VALUES (
    sqlc.arg(channel_id),
    sqlc.arg(user_id),
    sqlc.arg(name),
    FALSE,
    'unknown'
)
ON CONFLICT (user_id, channel_id) DO UPDATE
SET name = EXCLUDED.name,
    updated_at = now()
RETURNING *;

-- name: UpdateBotSession :execrows
UPDATE /* TEMPLATE: schema */bots
SET session = sqlc.arg(session),
    updated_at = now()
WHERE user_id = sqlc.arg(user_id)
  AND bot_id = sqlc.arg(bot_id);

-- name: MarkBotUploadSuccess :execrows
UPDATE /* TEMPLATE: schema */bots
SET consecutive_failures = 0,
    last_error = NULL,
    last_used_at = now(),
    retry_after = NULL,
    updated_at = now()
WHERE user_id = sqlc.arg(user_id)
  AND bot_id = sqlc.arg(bot_id);

-- name: MarkBotUploadFailure :execrows
UPDATE /* TEMPLATE: schema */bots
SET consecutive_failures = consecutive_failures + 1,
    last_error = left(sqlc.arg(last_error), 1000),
    retry_after = now() + interval '30 seconds',
    updated_at = now()
WHERE user_id = sqlc.arg(user_id)
  AND bot_id = sqlc.arg(bot_id);
