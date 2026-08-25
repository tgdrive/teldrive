-- name: CreateUploadSession :one
INSERT INTO /* TEMPLATE: schema */upload_sessions (
    id,
    user_id,
    parent_id,
    name,
    normalized_name,
    expected_size,
    expected_hash_algorithm,
    expected_hash_value,
    mime_type,
    mod_time,
    encryption,
    encryption_key_version,
    conflict_policy,
    part_size,
    state,
    expires_at
) VALUES (
    sqlc.arg(id),
    sqlc.arg(user_id),
    sqlc.narg(parent_id),
    sqlc.arg(name),
    sqlc.arg(normalized_name),
    sqlc.arg(expected_size),
    sqlc.narg(expected_hash_algorithm),
    sqlc.narg(expected_hash_value),
    sqlc.narg(mime_type),
    sqlc.arg(mod_time),
    sqlc.arg(encryption),
    sqlc.narg(encryption_key_version),
    sqlc.arg(conflict_policy),
    sqlc.arg(part_size),
    'open',
    sqlc.arg(expires_at)
)
RETURNING *;

-- name: GetUploadSessionForUser :one
SELECT *
FROM /* TEMPLATE: schema */upload_sessions
WHERE id = sqlc.arg(upload_id)
  AND user_id = sqlc.arg(user_id);


-- name: GetUploadSessionAnyOwner :one
SELECT *
FROM /* TEMPLATE: schema */upload_sessions
WHERE id = sqlc.arg(upload_id);

-- name: ListUploadSessions :many
SELECT *
FROM /* TEMPLATE: schema */upload_sessions
WHERE user_id = sqlc.arg(user_id)
  AND (sqlc.narg(state)::/* TEMPLATE: schema */upload_state IS NULL OR state = sqlc.narg(state)::/* TEMPLATE: schema */upload_state)
  AND (
    sqlc.narg(after_created_at)::timestamptz IS NULL
    OR (created_at, id) < (
      sqlc.narg(after_created_at)::timestamptz,
      sqlc.narg(after_id)::uuid
    )
  )
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(page_size);

-- name: GetUploadPart :one
SELECT *
FROM /* TEMPLATE: schema */upload_parts
WHERE upload_id = sqlc.arg(upload_id)
  AND part_no = sqlc.arg(part_no);

-- name: ClaimUploadPart :one
INSERT INTO /* TEMPLATE: schema */upload_parts (
    upload_id,
    part_no,
    channel_id,
    plain_size,
    checksum,
    state,
    lease_token,
    lease_expires_at
) VALUES (
    sqlc.arg(upload_id),
    sqlc.arg(part_no),
    sqlc.arg(channel_id),
    sqlc.arg(plain_size),
    sqlc.narg(checksum),
    'uploading',
    sqlc.arg(lease_token),
    sqlc.arg(lease_expires_at)
)
ON CONFLICT (upload_id, part_no) DO UPDATE
SET channel_id = EXCLUDED.channel_id,
    plain_size = EXCLUDED.plain_size,
    checksum = EXCLUDED.checksum,
    state = 'uploading',
    lease_token = EXCLUDED.lease_token,
    lease_expires_at = EXCLUDED.lease_expires_at,
    last_error_code = NULL,
    updated_at = now()
WHERE /* TEMPLATE: schema */upload_parts.state <> 'stored'
  AND (
    /* TEMPLATE: schema */upload_parts.lease_expires_at IS NULL
    OR /* TEMPLATE: schema */upload_parts.lease_expires_at < now()
  )
RETURNING *;

-- name: MarkUploadPartStored :one
UPDATE /* TEMPLATE: schema */upload_parts
SET message_id = sqlc.arg(message_id),
    stored_size = sqlc.arg(stored_size),
    checksum = sqlc.arg(checksum),
    salt = sqlc.narg(salt),
    block_hashes = sqlc.narg(block_hashes),
    state = 'stored',
    lease_token = NULL,
    lease_expires_at = NULL,
    last_error_code = NULL,
    updated_at = now()
WHERE upload_id = sqlc.arg(upload_id)
  AND part_no = sqlc.arg(part_no)
  AND state = 'uploading'
  AND lease_token = sqlc.arg(lease_token)
RETURNING *;

-- name: RenewUploadPartLease :execrows
UPDATE /* TEMPLATE: schema */upload_parts
SET lease_expires_at = sqlc.arg(lease_expires_at),
    updated_at = now()
WHERE upload_id = sqlc.arg(upload_id)
  AND part_no = sqlc.arg(part_no)
  AND state = 'uploading'
  AND lease_token = sqlc.arg(lease_token);

-- name: MarkUploadPartFailed :one
UPDATE /* TEMPLATE: schema */upload_parts
SET state = 'failed',
    lease_token = NULL,
    lease_expires_at = NULL,
    last_error_code = sqlc.arg(error_code),
    updated_at = now()
WHERE upload_id = sqlc.arg(upload_id)
  AND part_no = sqlc.arg(part_no)
  AND lease_token = sqlc.arg(lease_token)
RETURNING *;

-- name: ListUploadParts :many
SELECT *
FROM /* TEMPLATE: schema */upload_parts
WHERE upload_id = sqlc.arg(upload_id)
  AND (
    sqlc.narg(after_part_no)::integer IS NULL
    OR part_no > sqlc.narg(after_part_no)::integer
  )
ORDER BY part_no
LIMIT sqlc.arg(page_size);

-- name: LockUploadSessionForCompletion :one
SELECT *
FROM /* TEMPLATE: schema */upload_sessions
WHERE id = sqlc.arg(upload_id)
  AND user_id = sqlc.arg(user_id)
FOR UPDATE;

-- name: MarkUploadCompleting :one
UPDATE /* TEMPLATE: schema */upload_sessions
SET state = 'completing',
    updated_at = now()
WHERE id = sqlc.arg(upload_id)
  AND user_id = sqlc.arg(user_id)
  AND state = 'open'
RETURNING *;

-- name: FinalizeUploadExpectedSize :one
UPDATE /* TEMPLATE: schema */upload_sessions
SET expected_size = sqlc.arg(expected_size),
    updated_at = now()
WHERE id = sqlc.arg(upload_id)
  AND user_id = sqlc.arg(user_id)
  AND expected_size = -1
  AND state = 'open'
RETURNING *;

-- name: GetStoredUploadPartSummary :one
SELECT
    count(*)::integer AS part_count,
    COALESCE(sum(plain_size), 0)::bigint AS plain_size,
    COALESCE(sum(stored_size), 0)::bigint AS stored_size,
    COALESCE(min(part_no), 0)::integer AS min_part_no,
    COALESCE(max(part_no), 0)::integer AS max_part_no
FROM /* TEMPLATE: schema */upload_parts
WHERE upload_id = sqlc.arg(upload_id)
  AND state = 'stored';

-- name: ListStoredUploadPartHashes :many
SELECT block_hashes
FROM /* TEMPLATE: schema */upload_parts
WHERE upload_id = sqlc.arg(upload_id)
  AND state = 'stored'
ORDER BY part_no;

-- name: InsertFileFromUpload :one
INSERT INTO /* TEMPLATE: schema */files (
    id,
    user_id,
    parent_id,
    name,
    normalized_name,
    kind,
    mime_type,
    size,
    hash_algorithm,
    hash_value,
    encryption,
    encryption_key_version,
    status,
    mod_time
)
SELECT
    sqlc.arg(file_id),
    user_id,
    parent_id,
    name,
    normalized_name,
    'file',
    mime_type,
    expected_size,
    sqlc.arg(hash_algorithm),
    sqlc.arg(hash_value),
    encryption,
    encryption_key_version,
    'active',
    mod_time
FROM /* TEMPLATE: schema */upload_sessions us
WHERE us.id = sqlc.arg(upload_id)
  AND us.user_id = sqlc.arg(user_id)
  AND us.state = 'completing'
RETURNING *;

-- name: InsertFilePartsFromUpload :execrows
INSERT INTO /* TEMPLATE: schema */file_parts (
    file_id,
    part_no,
    channel_id,
    message_id,
    plain_size,
    stored_size,
    checksum,
    salt,
    block_hashes
)
SELECT
    sqlc.arg(file_id),
    part_no,
    channel_id,
    message_id,
    plain_size,
    stored_size,
    checksum,
    salt,
    block_hashes
FROM /* TEMPLATE: schema */upload_parts
WHERE upload_id = sqlc.arg(upload_id)
  AND state = 'stored'
ORDER BY part_no;

-- name: CompleteUploadSession :one
UPDATE /* TEMPLATE: schema */upload_sessions
SET state = 'completed',
    file_id = sqlc.arg(file_id),
    completed_at = now(),
    updated_at = now()
WHERE id = sqlc.arg(upload_id)
  AND user_id = sqlc.arg(user_id)
  AND state = 'completing'
RETURNING *;

-- name: AbortUploadSession :one
UPDATE /* TEMPLATE: schema */upload_sessions
SET state = 'aborted',
    updated_at = now()
WHERE id = sqlc.arg(upload_id)
  AND user_id = sqlc.arg(user_id)
  AND state IN ('open', 'completing')
RETURNING *;

-- name: ExpireUploadSessions :many
UPDATE /* TEMPLATE: schema */upload_sessions
SET state = 'expired',
    updated_at = now()
WHERE id IN (
    SELECT id
    FROM /* TEMPLATE: schema */upload_sessions
    WHERE state IN ('open', 'completing')
      AND expires_at <= now()
    ORDER BY expires_at
    FOR UPDATE SKIP LOCKED
    LIMIT sqlc.arg(batch_size)
)
RETURNING *;

-- name: ListUploadPartsForCleanup :many
SELECT up.*
FROM /* TEMPLATE: schema */upload_parts up
JOIN /* TEMPLATE: schema */upload_sessions us ON us.id = up.upload_id
WHERE us.id = sqlc.arg(upload_id)
  AND us.state IN ('aborted', 'expired')
  AND up.message_id IS NOT NULL
ORDER BY up.part_no;

-- name: ListUploadSessionsPendingCleanup :many
SELECT DISTINCT us.*
FROM /* TEMPLATE: schema */upload_sessions us
JOIN /* TEMPLATE: schema */upload_parts up ON up.upload_id = us.id
WHERE us.state IN ('aborted', 'expired')
  AND up.message_id IS NOT NULL
ORDER BY us.updated_at, us.id
LIMIT sqlc.arg(batch_size);

-- name: DeleteUploadPartForCleanup :execrows
DELETE FROM /* TEMPLATE: schema */upload_parts up
USING /* TEMPLATE: schema */upload_sessions us
WHERE up.upload_id = sqlc.arg(upload_id)
  AND up.part_no = sqlc.arg(part_no)
  AND up.message_id = sqlc.arg(message_id)
  AND us.id = up.upload_id
  AND us.state IN ('aborted', 'expired');

-- name: LockUploadDestinationConflict :one
SELECT *
FROM /* TEMPLATE: schema */files
WHERE user_id = sqlc.arg(user_id)
  AND parent_id IS NOT DISTINCT FROM sqlc.narg(parent_id)::uuid
  AND normalized_name = sqlc.arg(normalized_name)
  AND status = 'active'
FOR UPDATE;

-- name: MarkActiveFileDeletionPendingForReplace :execrows
UPDATE /* TEMPLATE: schema */files
SET status = 'deletion_pending',
    deleted_at = COALESCE(deleted_at, now()),
    updated_at = now(),
    generation = generation + 1
WHERE id = sqlc.arg(file_id)
  AND user_id = sqlc.arg(user_id)
  AND status = 'active';

-- name: RevokeActiveSharesForFile :exec
UPDATE /* TEMPLATE: schema */file_shares
SET revoked_at = COALESCE(revoked_at, now())
WHERE file_id = sqlc.arg(file_id)
  AND owner_id = sqlc.arg(user_id)
  AND revoked_at IS NULL;

-- name: RenameUploadSession :execrows
UPDATE /* TEMPLATE: schema */upload_sessions
SET name = sqlc.arg(name),
    normalized_name = sqlc.arg(normalized_name),
    updated_at = now()
WHERE id = sqlc.arg(upload_id)
  AND user_id = sqlc.arg(user_id);

-- name: GetAllUploadPartSummary :one
SELECT
    count(*)::bigint AS total_parts,
    count(*) FILTER (WHERE state = 'stored')::bigint AS stored_parts,
    COALESCE(sum(plain_size) FILTER (WHERE state = 'stored'), 0)::bigint AS stored_plain_size,
    COALESCE(min(part_no) FILTER (WHERE state = 'stored'), 0)::integer AS min_part_no,
    COALESCE(max(part_no) FILTER (WHERE state = 'stored'), 0)::integer AS max_part_no
FROM /* TEMPLATE: schema */upload_parts
WHERE upload_id = sqlc.arg(upload_id);

-- name: CountInvalidOpenEndedUploadParts :one
WITH final_part AS (
    SELECT COALESCE(max(part_no), 0)::integer AS part_no
    FROM /* TEMPLATE: schema */upload_parts
    WHERE upload_id = sqlc.arg(upload_id)
      AND state = 'stored'
)
SELECT count(*)::bigint
FROM /* TEMPLATE: schema */upload_parts parts
CROSS JOIN final_part
WHERE parts.upload_id = sqlc.arg(upload_id)
  AND parts.state = 'stored'
  AND parts.part_no < final_part.part_no
  AND parts.plain_size <> sqlc.arg(part_size);

-- name: ListUploadDailyStatistics :many
WITH days AS (
  SELECT generate_series(
    (CURRENT_DATE - (sqlc.arg(days)::integer - 1))::date,
    CURRENT_DATE,
    interval '1 day'
  )::date AS day
), totals AS (
  SELECT completed_at::date AS day,
         COALESCE(sum(expected_size), 0)::bigint AS uploaded_bytes,
         count(*)::bigint AS completed_files
  FROM /* TEMPLATE: schema */upload_sessions
  WHERE user_id = sqlc.arg(user_id)
    AND state = 'completed'
    AND completed_at >= CURRENT_DATE - (sqlc.arg(days)::integer - 1)
  GROUP BY completed_at::date
)
SELECT d.day, COALESCE(t.uploaded_bytes, 0)::bigint AS uploaded_bytes,
       COALESCE(t.completed_files, 0)::bigint AS completed_files
FROM days d
LEFT JOIN totals t USING (day)
ORDER BY d.day;
