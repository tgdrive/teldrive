-- name: AcquireAdvisoryLock :exec
SELECT pg_advisory_lock(sqlc.arg(lock_id));

-- name: ReleaseAdvisoryLock :one
SELECT pg_advisory_unlock(sqlc.arg(lock_id));

-- name: TryAdvisoryLock :one
SELECT pg_try_advisory_lock(sqlc.arg(lock_id));

-- name: TryAdvisoryLocks :many
SELECT candidate.lock_id::bigint AS lock_id, pg_try_advisory_lock(candidate.lock_id) AS locked
FROM unnest(sqlc.arg(lock_ids)::bigint[]) AS candidate(lock_id);

-- name: AcquireAdvisoryTransactionLock :exec
SELECT pg_advisory_xact_lock(sqlc.arg(lock_id));
