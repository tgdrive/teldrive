-- name: ListUserEventsAfter :many
SELECT id, user_id, event_type, resource_type, resource_id, generation, payload, occurred_at
FROM /* TEMPLATE: schema */user_events
WHERE user_id = sqlc.arg(user_id)
  AND id > sqlc.arg(after_id)
  AND (
    COALESCE(cardinality(sqlc.arg(event_types)::text[]), 0) = 0
    OR event_type = ANY(sqlc.arg(event_types)::text[])
  )
ORDER BY id
LIMIT sqlc.arg(event_limit);

-- name: GetUserEventCursorState :one
SELECT
    EXISTS (
        SELECT 1
        FROM /* TEMPLATE: schema */user_events AS cursor_event
        WHERE cursor_event.user_id = sqlc.arg(cursor_user_id)
          AND cursor_event.id = sqlc.arg(after_id)
    ) AS cursor_exists,
    COALESCE(MIN(event_rows.id), 0)::bigint AS oldest_id,
    COALESCE(MAX(event_rows.id), 0)::bigint AS newest_id,
    COALESCE((
        SELECT stream_state.last_event_id
        FROM /* TEMPLATE: schema */user_event_stream_state AS stream_state
        WHERE stream_state.user_id = sqlc.arg(cursor_user_id)
    ), 0)::bigint AS last_event_id
FROM /* TEMPLATE: schema */user_events AS event_rows
WHERE event_rows.user_id = sqlc.arg(cursor_user_id);

-- name: InsertUserEvent :one
INSERT INTO /* TEMPLATE: schema */user_events (
    user_id, event_type, resource_type, resource_id, generation, payload
) VALUES (
    sqlc.arg(user_id),
    sqlc.arg(event_type),
    sqlc.arg(resource_type),
    sqlc.narg(resource_id),
    sqlc.narg(generation),
    sqlc.arg(payload)
)
RETURNING id, user_id, event_type, resource_type, resource_id, generation, payload, occurred_at;

-- name: DeleteUserEventsBefore :execrows
DELETE FROM /* TEMPLATE: schema */user_events
WHERE occurred_at < sqlc.arg(cutoff);

-- name: CreateEventStreamTicket :exec
INSERT INTO /* TEMPLATE: schema */event_stream_tickets (token_hash, user_id, expires_at)
VALUES (sqlc.arg(token_hash), sqlc.arg(user_id), sqlc.arg(expires_at));

-- name: GetEventStreamTicketUser :one
SELECT user_id
FROM /* TEMPLATE: schema */event_stream_tickets
WHERE token_hash = sqlc.arg(token_hash)
  AND expires_at > now();

-- name: DeleteExpiredEventStreamTickets :execrows
DELETE FROM /* TEMPLATE: schema */event_stream_tickets
WHERE expires_at <= now();
