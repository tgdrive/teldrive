-- name: AcquireAdvisoryLock :exec
SELECT pg_advisory_lock(sqlc.arg(lock_id));

-- name: ReleaseAdvisoryLock :one
SELECT pg_advisory_unlock(sqlc.arg(lock_id));

-- name: TryAdvisoryLock :one
SELECT pg_try_advisory_lock(sqlc.arg(lock_id));

-- name: AcquireAdvisoryTransactionLock :exec
SELECT pg_advisory_xact_lock(sqlc.arg(lock_id));
