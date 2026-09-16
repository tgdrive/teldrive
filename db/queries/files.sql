-- name: GetFileForUser :one
SELECT *
FROM /* TEMPLATE: schema */files
WHERE id = sqlc.arg(file_id)
  AND user_id = sqlc.arg(user_id);

-- name: GetActiveFolderForUser :one
SELECT *
FROM /* TEMPLATE: schema */files
WHERE id = sqlc.arg(folder_id)
  AND user_id = sqlc.arg(user_id)
  AND kind = 'folder'
  AND status = 'active';

-- name: ListFiles :many
SELECT *
FROM /* TEMPLATE: schema */files
WHERE files.user_id = sqlc.arg(user_id)
  AND (
    files.parent_id IS NOT DISTINCT FROM sqlc.narg(parent_id)::uuid
    OR (
      sqlc.arg(status)::/* TEMPLATE: schema */file_status = 'trashed'
      AND sqlc.narg(parent_id)::uuid IS NULL
      AND files.parent_id IS NOT NULL
      AND NOT EXISTS (
        SELECT 1
        FROM /* TEMPLATE: schema */files parent
        WHERE parent.id = files.parent_id
          AND parent.user_id = files.user_id
          AND parent.status = 'trashed'
      )
    )
  )
  AND files.status = sqlc.arg(status)::/* TEMPLATE: schema */file_status
  AND (sqlc.narg(kind)::/* TEMPLATE: schema */file_kind IS NULL OR files.kind = sqlc.narg(kind)::/* TEMPLATE: schema */file_kind)
  AND (
    sqlc.narg(search)::text IS NULL
    OR files.normalized_name % sqlc.narg(search)::text
    OR files.normalized_name ILIKE '%' || sqlc.narg(search)::text || '%'
  )
  AND (
    sqlc.narg(after_name)::text IS NULL
    OR (normalized_name, id) > (sqlc.narg(after_name)::text, sqlc.narg(after_id)::uuid)
  )
ORDER BY normalized_name, id
LIMIT sqlc.arg(page_size);

-- name: CreateFolder :one
INSERT INTO /* TEMPLATE: schema */files (
    id,
    user_id,
    parent_id,
    name,
    normalized_name,
    kind,
    mime_type,
    size,
    encryption,
    status,
    mod_time
) VALUES (
    sqlc.arg(id),
    sqlc.arg(user_id),
    sqlc.narg(parent_id),
    sqlc.arg(name),
    sqlc.arg(normalized_name),
    'folder',
    'inode/directory',
    NULL,
    false,
    'active',
    sqlc.arg(mod_time)
)
RETURNING *;

-- name: UpdateFileMetadata :one
UPDATE /* TEMPLATE: schema */files
SET name = COALESCE(sqlc.narg(name), name),
    normalized_name = COALESCE(sqlc.narg(normalized_name), normalized_name),
    mod_time = COALESCE(sqlc.narg(mod_time), mod_time),
    generation = generation + 1,
    updated_at = now()
WHERE id = sqlc.arg(file_id)
  AND user_id = sqlc.arg(user_id)
  AND status = 'active'
  AND (
    sqlc.narg(expected_generation)::bigint IS NULL
    OR generation = sqlc.narg(expected_generation)::bigint
  )
RETURNING *;

-- name: MoveFile :one
UPDATE /* TEMPLATE: schema */files
SET parent_id = sqlc.narg(parent_id),
    generation = generation + 1,
    updated_at = now()
WHERE id = sqlc.arg(file_id)
  AND user_id = sqlc.arg(user_id)
  AND status = 'active'
  AND (
    sqlc.narg(expected_generation)::bigint IS NULL
    OR generation = sqlc.narg(expected_generation)::bigint
  )
RETURNING *;

-- name: TrashFile :one
UPDATE /* TEMPLATE: schema */files
SET status = 'trashed',
    deleted_at = now(),
    generation = generation + 1,
    updated_at = now()
WHERE id = sqlc.arg(file_id)
  AND user_id = sqlc.arg(user_id)
  AND status = 'active'
RETURNING *;

-- name: RestoreFileSubtree :many
WITH RECURSIVE target AS (
  SELECT root.id
  FROM /* TEMPLATE: schema */files root
  WHERE root.id = sqlc.arg(file_id)
    AND root.user_id = sqlc.arg(user_id)
    AND root.status = 'trashed'
    AND (
      root.parent_id IS NULL
      OR EXISTS (
        SELECT 1
        FROM /* TEMPLATE: schema */files parent
        WHERE parent.id = root.parent_id
          AND parent.user_id = root.user_id
          AND parent.status = 'active'
      )
    )
  UNION ALL
  SELECT child.id
  FROM /* TEMPLATE: schema */files child
  JOIN target parent ON child.parent_id = parent.id
  WHERE child.user_id = sqlc.arg(user_id)
    AND child.status = 'trashed'
)
UPDATE /* TEMPLATE: schema */files AS target_file
SET status = 'active',
    deleted_at = NULL,
    generation = target_file.generation + 1,
    updated_at = now()
WHERE target_file.user_id = sqlc.arg(user_id)
  AND target_file.id IN (SELECT target.id FROM target)
RETURNING target_file.*;

-- name: MarkFileDeletionPending :one
UPDATE /* TEMPLATE: schema */files
SET status = 'deletion_pending',
    deleted_at = COALESCE(deleted_at, now()),
    generation = generation + 1,
    updated_at = now()
WHERE id = sqlc.arg(file_id)
  AND user_id = sqlc.arg(user_id)
  AND status = 'trashed'
RETURNING *;

-- name: DeleteFileCatalogRowsByIDs :execrows
DELETE FROM /* TEMPLATE: schema */files
WHERE id = ANY(sqlc.arg(file_ids)::uuid[])
  AND user_id = sqlc.arg(user_id)
  AND status = 'deletion_pending';

-- Recursive move-cycle validation will be implemented as a hand-reviewed query in the file service.

-- name: ListFileParts :many
SELECT *
FROM /* TEMPLATE: schema */file_parts
WHERE file_id = sqlc.arg(file_id)
ORDER BY part_no;

-- name: ListFilePartsByFileIDs :many
SELECT *
FROM /* TEMPLATE: schema */file_parts
WHERE file_id = ANY(sqlc.arg(file_ids)::uuid[])
ORDER BY file_id, part_no;

-- name: SumFilePartSizes :one
SELECT
    COALESCE(sum(plain_size), 0)::bigint AS plain_size,
    COALESCE(sum(stored_size), 0)::bigint AS stored_size,
    count(*)::integer AS part_count
FROM /* TEMPLATE: schema */file_parts
WHERE file_id = sqlc.arg(file_id);

-- name: ResolveActiveChildFolder :one
SELECT id
FROM /* TEMPLATE: schema */files
WHERE user_id = sqlc.arg(user_id)
  AND parent_id IS NOT DISTINCT FROM sqlc.narg(parent_id)::uuid
  AND normalized_name = sqlc.arg(normalized_name)
  AND kind = 'folder'
  AND status = 'active';

-- name: ResolveActiveChild :one
SELECT *
FROM /* TEMPLATE: schema */files
WHERE user_id = sqlc.arg(user_id)
  AND parent_id IS NOT DISTINCT FROM sqlc.narg(parent_id)::uuid
  AND normalized_name = sqlc.arg(normalized_name)
  AND status = 'active';

-- name: ListFilesAdvanced :many
SELECT f.*
FROM /* TEMPLATE: schema */files f
WHERE f.user_id = sqlc.arg(user_id)
  AND (
    f.parent_id IS NOT DISTINCT FROM sqlc.narg(parent_id)::uuid
    OR (
      sqlc.arg(status)::/* TEMPLATE: schema */file_status = 'trashed'
      AND sqlc.narg(parent_id)::uuid IS NULL
      AND f.parent_id IS NOT NULL
      AND NOT EXISTS (
        SELECT 1
        FROM /* TEMPLATE: schema */files parent
        WHERE parent.id = f.parent_id
          AND parent.user_id = f.user_id
          AND parent.status = 'trashed'
      )
    )
  )
  AND f.status = sqlc.arg(status)::/* TEMPLATE: schema */file_status
  AND (sqlc.narg(kind)::/* TEMPLATE: schema */file_kind IS NULL OR f.kind = sqlc.narg(kind)::/* TEMPLATE: schema */file_kind)
  AND (
    sqlc.narg(search)::text IS NULL
    OR (sqlc.arg(search_type)::text = 'regex' AND f.name ~* sqlc.narg(search)::text)
    OR (
      sqlc.arg(search_type)::text = 'text'
      AND (
        f.normalized_name % sqlc.narg(search)::text
        OR f.normalized_name ILIKE '%' || sqlc.narg(search)::text || '%'
      )
    )
  )
  AND (
    cardinality(sqlc.arg(categories)::text[]) = 0
    OR (CASE
      WHEN f.kind = 'folder' THEN 'other'
      WHEN lower(COALESCE(f.mime_type, '')) LIKE 'image/%' THEN 'image'
      WHEN lower(COALESCE(f.mime_type, '')) LIKE 'audio/%' THEN 'audio'
      WHEN lower(COALESCE(f.mime_type, '')) LIKE 'video/%' THEN 'video'
      WHEN lower(COALESCE(f.mime_type, '')) LIKE 'text/%'
        OR lower(COALESCE(f.mime_type, '')) IN (
          'application/pdf', 'application/json', 'application/xml',
          'application/msword', 'application/rtf',
          'application/vnd.ms-excel', 'application/vnd.ms-powerpoint',
          'application/vnd.openxmlformats-officedocument.wordprocessingml.document',
          'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
          'application/vnd.openxmlformats-officedocument.presentationml.presentation'
        ) THEN 'document'
      WHEN lower(COALESCE(f.mime_type, '')) IN (
          'application/zip', 'application/x-rar-compressed', 'application/x-7z-compressed',
          'application/x-tar', 'application/gzip', 'application/x-bzip2', 'application/x-xz'
        ) OR lower(f.name) ~ '\.(zip|rar|7z|tar|gz|tgz|bz2|xz)$' THEN 'archive'
      ELSE 'other'
    END) = ANY(sqlc.arg(categories)::text[])
  )
  AND (sqlc.narg(updated_after)::timestamptz IS NULL OR f.updated_at >= sqlc.narg(updated_after)::timestamptz)
  AND (sqlc.narg(updated_before)::timestamptz IS NULL OR f.updated_at < sqlc.narg(updated_before)::timestamptz)
  AND (
    sqlc.narg(after_id)::uuid IS NULL
    OR (
      sqlc.arg(sort_by)::text = 'name'
      AND sqlc.narg(after_name)::text IS NOT NULL
      AND (
        (sqlc.arg(sort_order)::text = 'asc' AND (f.normalized_name, f.id) > (sqlc.narg(after_name)::text, sqlc.narg(after_id)::uuid))
        OR (sqlc.arg(sort_order)::text = 'desc' AND (f.normalized_name, f.id) < (sqlc.narg(after_name)::text, sqlc.narg(after_id)::uuid))
      )
    )
    OR (
      sqlc.arg(sort_by)::text = 'updatedAt'
      AND sqlc.narg(after_updated_at)::timestamptz IS NOT NULL
      AND (
        (sqlc.arg(sort_order)::text = 'asc' AND (f.updated_at, f.id) > (sqlc.narg(after_updated_at)::timestamptz, sqlc.narg(after_id)::uuid))
        OR (sqlc.arg(sort_order)::text = 'desc' AND (f.updated_at, f.id) < (sqlc.narg(after_updated_at)::timestamptz, sqlc.narg(after_id)::uuid))
      )
    )
    OR (
      sqlc.arg(sort_by)::text = 'size'
      AND sqlc.narg(after_size)::bigint IS NOT NULL
      AND (
        (sqlc.arg(sort_order)::text = 'asc' AND (COALESCE(f.size, -1), f.id) > (sqlc.narg(after_size)::bigint, sqlc.narg(after_id)::uuid))
        OR (sqlc.arg(sort_order)::text = 'desc' AND (COALESCE(f.size, -1), f.id) < (sqlc.narg(after_size)::bigint, sqlc.narg(after_id)::uuid))
      )
    )
    OR (
      sqlc.arg(sort_by)::text = 'id'
      AND (
        (sqlc.arg(sort_order)::text = 'asc' AND f.id > sqlc.narg(after_id)::uuid)
        OR (sqlc.arg(sort_order)::text = 'desc' AND f.id < sqlc.narg(after_id)::uuid)
      )
    )
  )
ORDER BY
  CASE WHEN sqlc.arg(sort_by)::text = 'name' AND sqlc.arg(sort_order)::text = 'asc' THEN f.normalized_name END ASC,
  CASE WHEN sqlc.arg(sort_by)::text = 'name' AND sqlc.arg(sort_order)::text = 'desc' THEN f.normalized_name END DESC,
  CASE WHEN sqlc.arg(sort_by)::text = 'updatedAt' AND sqlc.arg(sort_order)::text = 'asc' THEN f.updated_at END ASC,
  CASE WHEN sqlc.arg(sort_by)::text = 'updatedAt' AND sqlc.arg(sort_order)::text = 'desc' THEN f.updated_at END DESC,
  CASE WHEN sqlc.arg(sort_by)::text = 'size' AND sqlc.arg(sort_order)::text = 'asc' THEN COALESCE(f.size, -1) END ASC,
  CASE WHEN sqlc.arg(sort_by)::text = 'size' AND sqlc.arg(sort_order)::text = 'desc' THEN COALESCE(f.size, -1) END DESC,
  CASE WHEN sqlc.arg(sort_by)::text = 'id' AND sqlc.arg(sort_order)::text = 'asc' THEN f.id END ASC,
  CASE WHEN sqlc.arg(sort_by)::text = 'id' AND sqlc.arg(sort_order)::text = 'desc' THEN f.id END DESC,
  CASE WHEN sqlc.arg(sort_order)::text = 'asc' THEN f.id END ASC,
  CASE WHEN sqlc.arg(sort_order)::text = 'desc' THEN f.id END DESC
LIMIT sqlc.arg(page_size);

-- name: ListFileCategoryStatistics :many
SELECT category, count(*)::bigint AS total_files, COALESCE(sum(size), 0)::bigint AS total_size
FROM (
  SELECT CASE
    WHEN lower(COALESCE(f.mime_type, '')) LIKE 'image/%' THEN 'image'
    WHEN lower(COALESCE(f.mime_type, '')) LIKE 'audio/%' THEN 'audio'
    WHEN lower(COALESCE(f.mime_type, '')) LIKE 'video/%' THEN 'video'
    WHEN lower(COALESCE(f.mime_type, '')) LIKE 'text/%'
      OR lower(COALESCE(f.mime_type, '')) IN (
        'application/pdf', 'application/json', 'application/xml',
        'application/msword', 'application/rtf',
        'application/vnd.ms-excel', 'application/vnd.ms-powerpoint',
        'application/vnd.openxmlformats-officedocument.wordprocessingml.document',
        'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
        'application/vnd.openxmlformats-officedocument.presentationml.presentation'
      ) THEN 'document'
    WHEN lower(COALESCE(f.mime_type, '')) IN (
        'application/zip', 'application/x-rar-compressed', 'application/x-7z-compressed',
        'application/x-tar', 'application/gzip', 'application/x-bzip2', 'application/x-xz'
      ) OR lower(f.name) ~ '\.(zip|rar|7z|tar|gz|tgz|bz2|xz)$' THEN 'archive'
    ELSE 'other'
  END AS category, f.size
  FROM /* TEMPLATE: schema */files f
  WHERE f.user_id = sqlc.arg(user_id)
    AND f.kind = 'file'
    AND f.status = 'active'
) categorized
GROUP BY category
ORDER BY category;

-- name: GetDriveStatistics :one
SELECT
  count(*) FILTER (WHERE kind = 'file' AND status = 'active')::bigint AS total_files,
  count(*) FILTER (WHERE kind = 'folder' AND status = 'active')::bigint AS total_folders,
  COALESCE(sum(size) FILTER (WHERE kind = 'file' AND status = 'active'), 0)::bigint AS total_bytes,
  count(*) FILTER (WHERE status = 'trashed')::bigint AS trashed_files,
  (SELECT count(*)::bigint FROM /* TEMPLATE: schema */file_shares WHERE owner_id = sqlc.arg(user_id) AND revoked_at IS NULL AND (expires_at IS NULL OR expires_at > now())) AS active_shares,
  (SELECT count(*)::bigint FROM /* TEMPLATE: schema */upload_sessions WHERE user_id = sqlc.arg(user_id) AND state IN ('open', 'completing')) AS open_uploads
FROM /* TEMPLATE: schema */files
WHERE user_id = sqlc.arg(user_id);

-- name: LockActiveFiles :many
SELECT *
FROM /* TEMPLATE: schema */files
WHERE user_id = sqlc.arg(user_id)
  AND id = ANY(sqlc.arg(file_ids)::uuid[])
  AND status = 'active'
FOR UPDATE;

-- name: LockActiveFolder :one
SELECT *
FROM /* TEMPLATE: schema */files
WHERE id = sqlc.arg(folder_id)
  AND user_id = sqlc.arg(user_id)
  AND kind = 'folder'
  AND status = 'active'
FOR UPDATE;

-- name: LockActiveDestinationEntries :many
SELECT id, normalized_name
FROM /* TEMPLATE: schema */files
WHERE user_id = sqlc.arg(user_id)
  AND parent_id IS NOT DISTINCT FROM sqlc.narg(parent_id)::uuid
  AND status = 'active'
FOR UPDATE;

-- name: ListFileAncestorIDs :many
WITH RECURSIVE ancestors AS (
  SELECT file.id, file.parent_id
  FROM /* TEMPLATE: schema */files AS file
  WHERE file.id = sqlc.arg(file_id)
    AND file.user_id = sqlc.arg(user_id)
  UNION ALL
  SELECT parent.id, parent.parent_id
  FROM /* TEMPLATE: schema */files AS parent
  JOIN ancestors AS child ON parent.id = child.parent_id
  WHERE parent.user_id = sqlc.arg(user_id)
)
SELECT id FROM ancestors;

-- name: ListFileSubtreeIDs :many
WITH RECURSIVE subtree AS (
  SELECT root.id FROM /* TEMPLATE: schema */files root WHERE root.id = sqlc.arg(file_id) AND root.user_id = sqlc.arg(user_id)
  UNION ALL
  SELECT f.id FROM /* TEMPLATE: schema */files f JOIN subtree s ON f.parent_id = s.id WHERE f.user_id = sqlc.arg(user_id)
)
SELECT id FROM subtree;

-- name: TrashFileSubtrees :many
WITH RECURSIVE target AS (
  SELECT root.id
  FROM /* TEMPLATE: schema */files root
  WHERE root.user_id = sqlc.arg(user_id)
    AND root.id = ANY(sqlc.arg(file_ids)::uuid[])
    AND root.status = 'active'
  UNION
  SELECT child.id
  FROM /* TEMPLATE: schema */files child
  JOIN target parent ON child.parent_id = parent.id
  WHERE child.user_id = sqlc.arg(user_id)
    AND child.status = 'active'
)
UPDATE /* TEMPLATE: schema */files AS target_file
SET status = 'trashed',
    deleted_at = now(),
    generation = target_file.generation + 1,
    updated_at = now()
WHERE target_file.user_id = sqlc.arg(user_id)
  AND target_file.id IN (SELECT target.id FROM target)
RETURNING target_file.*;

-- name: RevokeSharesForFileSubtrees :exec
WITH RECURSIVE target AS (
  SELECT root.id FROM /* TEMPLATE: schema */files root WHERE root.user_id = sqlc.arg(user_id) AND root.id = ANY(sqlc.arg(file_ids)::uuid[])
  UNION
  SELECT child.id FROM /* TEMPLATE: schema */files child JOIN target parent ON child.parent_id = parent.id WHERE child.user_id = sqlc.arg(user_id)
)
UPDATE /* TEMPLATE: schema */file_shares AS share
SET revoked_at = COALESCE(share.revoked_at, now())
WHERE share.owner_id = sqlc.arg(user_id)
  AND share.revoked_at IS NULL
  AND share.file_id IN (SELECT target.id FROM target);

-- name: MarkFileSubtreeDeletionPending :exec
WITH RECURSIVE target AS (
  SELECT root.id FROM /* TEMPLATE: schema */files root WHERE root.id = sqlc.arg(file_id) AND root.user_id = sqlc.arg(user_id) AND root.status = 'active'
  UNION ALL
  SELECT child.id FROM /* TEMPLATE: schema */files child JOIN target parent ON child.parent_id = parent.id WHERE child.user_id = sqlc.arg(user_id) AND child.status = 'active'
)
UPDATE /* TEMPLATE: schema */files AS target_file
SET status = 'deletion_pending',
    deleted_at = COALESCE(target_file.deleted_at, now()),
    generation = target_file.generation + 1,
    updated_at = now()
WHERE target_file.user_id = sqlc.arg(user_id) AND target_file.id IN (SELECT target.id FROM target);

-- name: MarkFileSubtreesDeletionPending :exec
WITH RECURSIVE target AS (
  SELECT root.id
  FROM /* TEMPLATE: schema */files AS root
  WHERE root.id = ANY(sqlc.arg(file_ids)::uuid[])
    AND root.user_id = sqlc.arg(user_id)
    AND root.status = 'active'
  UNION
  SELECT child.id
  FROM /* TEMPLATE: schema */files AS child
  JOIN target AS parent ON child.parent_id = parent.id
  WHERE child.user_id = sqlc.arg(user_id)
    AND child.status = 'active'
)
UPDATE /* TEMPLATE: schema */files AS target_file
SET status = 'deletion_pending',
    deleted_at = COALESCE(target_file.deleted_at, now()),
    generation = target_file.generation + 1,
    updated_at = now()
WHERE target_file.user_id = sqlc.arg(user_id)
  AND target_file.id IN (SELECT target.id FROM target);

-- name: RevokeSharesForFileSubtree :exec
WITH RECURSIVE target AS (
  SELECT root.id FROM /* TEMPLATE: schema */files root WHERE root.id = sqlc.arg(file_id) AND root.user_id = sqlc.arg(user_id)
  UNION ALL
  SELECT child.id FROM /* TEMPLATE: schema */files child JOIN target parent ON child.parent_id = parent.id WHERE child.user_id = sqlc.arg(user_id)
)
UPDATE /* TEMPLATE: schema */file_shares AS share
SET revoked_at = COALESCE(share.revoked_at, now())
WHERE share.owner_id = sqlc.arg(user_id)
  AND share.revoked_at IS NULL
  AND share.file_id IN (SELECT target.id FROM target);

-- name: ListActiveNormalizedNames :many
SELECT normalized_name
FROM /* TEMPLATE: schema */files
WHERE user_id = sqlc.arg(user_id)
  AND parent_id IS NOT DISTINCT FROM sqlc.narg(parent_id)::uuid
  AND status = 'active'
  AND (sqlc.narg(exclude_id)::uuid IS NULL OR id <> sqlc.narg(exclude_id)::uuid);

-- name: MoveFilesWithNames :many
WITH arrays AS (
  SELECT sqlc.arg(file_ids)::uuid[] AS file_ids,
         sqlc.arg(names)::text[] AS names,
         sqlc.arg(normalized_names)::text[] AS normalized_names
), input AS (
  SELECT arrays.file_ids[index] AS file_id,
         arrays.names[index] AS name,
         arrays.normalized_names[index] AS normalized_name
  FROM arrays
  CROSS JOIN LATERAL generate_subscripts(arrays.file_ids, 1) AS index
)
UPDATE /* TEMPLATE: schema */files AS file
SET parent_id = sqlc.narg(parent_id),
    name = input.name,
    normalized_name = input.normalized_name,
    generation = file.generation + 1,
    updated_at = now()
FROM input
WHERE file.id = input.file_id
  AND file.user_id = sqlc.arg(user_id)
  AND file.status = 'active'
  AND (sqlc.narg(expected_generation)::bigint IS NULL OR file.generation = sqlc.narg(expected_generation)::bigint)
RETURNING file.*;

-- name: LoadFileSubtree :many
WITH RECURSIVE tree AS (
    SELECT f.*, 0::integer AS depth
    FROM /* TEMPLATE: schema */files f
    WHERE f.id = sqlc.arg(root_id) AND f.user_id = sqlc.arg(user_id)
    UNION ALL
    SELECT child.*, tree.depth + 1
    FROM /* TEMPLATE: schema */files child
    JOIN tree ON child.parent_id = tree.id
    WHERE child.user_id = sqlc.arg(user_id)
)
SELECT id, user_id, parent_id, name, normalized_name, kind, mime_type, size,
       hash_algorithm, hash_value, encryption, encryption_key_version, status,
       mod_time, generation, created_at, updated_at, deleted_at, depth
FROM tree
ORDER BY depth, id;

-- name: LoadFileSubtrees :many
WITH RECURSIVE tree AS (
    SELECT f.*, 0::integer AS depth
    FROM /* TEMPLATE: schema */files f
    WHERE f.id = ANY(sqlc.arg(root_ids)::uuid[])
      AND f.user_id = sqlc.arg(user_id)
    UNION ALL
    SELECT child.*, tree.depth + 1
    FROM /* TEMPLATE: schema */files child
    JOIN tree ON child.parent_id = tree.id
    WHERE child.user_id = sqlc.arg(user_id)
)
SELECT id, user_id, parent_id, name, normalized_name, kind, mime_type, size,
       hash_algorithm, hash_value, encryption, encryption_key_version, status,
       mod_time, generation, created_at, updated_at, deleted_at, depth
FROM tree
ORDER BY depth, id;

-- name: InsertCopiedFiles :many
INSERT INTO /* TEMPLATE: schema */files AS file (
    id, user_id, parent_id, name, normalized_name, kind, mime_type, size,
    hash_algorithm, hash_value, encryption, encryption_key_version,
    status, mod_time, generation
)
SELECT input.id, input.user_id, input.parent_id, input.name, input.normalized_name,
       input.kind::/* TEMPLATE: schema */file_kind, input.mime_type, input.size,
       input.hash_algorithm, input.hash_value, input.encryption,
       input.encryption_key_version, 'active', input.mod_time, 1
FROM jsonb_to_recordset(sqlc.arg(files)::jsonb) AS input(
    id uuid, user_id bigint, parent_id uuid, name text, normalized_name text,
    kind text, mime_type text, size bigint, hash_algorithm text, hash_value text,
    encryption boolean, encryption_key_version integer, mod_time timestamptz
)
RETURNING file.*;

-- name: InsertCopiedFileParts :execrows
INSERT INTO /* TEMPLATE: schema */file_parts (
    file_id, part_no, channel_id, message_id, plain_size, stored_size,
    checksum, salt, block_hashes
)
SELECT input.file_id, input.part_no, input.channel_id, input.message_id,
       input.plain_size, input.stored_size, input.checksum, input.salt,
       decode(input.block_hashes, 'base64')
FROM jsonb_to_recordset(sqlc.arg(parts)::jsonb) AS input(
    file_id uuid, part_no integer, channel_id bigint, message_id bigint,
    plain_size bigint, stored_size bigint, checksum text, salt text,
    block_hashes text
);

-- name: MarkFileIDsDeletionPending :exec
UPDATE /* TEMPLATE: schema */files
SET status = 'deletion_pending',
    deleted_at = COALESCE(deleted_at, now()),
    updated_at = now()
WHERE user_id = sqlc.arg(user_id)
  AND id = ANY(sqlc.arg(file_ids)::uuid[]);

-- name: QueueFileSubtreePurge :many
WITH RECURSIVE target AS (
  SELECT root.id
  FROM /* TEMPLATE: schema */files root
  WHERE root.id = sqlc.arg(file_id)
    AND root.user_id = sqlc.arg(user_id)
    AND root.status = 'trashed'
  UNION ALL
  SELECT child.id
  FROM /* TEMPLATE: schema */files child
  JOIN target parent ON child.parent_id = parent.id
  WHERE child.user_id = sqlc.arg(user_id)
)
UPDATE /* TEMPLATE: schema */files AS target_file
SET status = 'deletion_pending',
    deleted_at = COALESCE(target_file.deleted_at, now()),
    updated_at = now()
WHERE target_file.user_id = sqlc.arg(user_id)
  AND target_file.id IN (SELECT target.id FROM target)
RETURNING target_file.*;


-- name: MarkAllTrashedDeletionPending :execrows
UPDATE /* TEMPLATE: schema */files
SET status = 'deletion_pending',
    deleted_at = COALESCE(deleted_at, now()),
    updated_at = now()
WHERE user_id = sqlc.arg(user_id)
  AND status = 'trashed';

-- name: ListFilePartMessageRefs :many
SELECT channel_id, message_id
FROM /* TEMPLATE: schema */file_parts
WHERE file_id = ANY(sqlc.arg(file_ids)::uuid[])
ORDER BY channel_id, message_id;

-- name: DeleteFilePartsByFileIDs :exec
DELETE FROM /* TEMPLATE: schema */file_parts
WHERE file_id = ANY(sqlc.arg(file_ids)::uuid[]);

-- name: ClearUploadSessionParentsByFileIDs :exec
UPDATE /* TEMPLATE: schema */upload_sessions
SET parent_id = NULL,
    updated_at = now()
WHERE user_id = sqlc.arg(user_id)
  AND parent_id = ANY(sqlc.arg(file_ids)::uuid[]);

-- name: UpdateFilePartSizes :execrows
UPDATE /* TEMPLATE: schema */file_parts
SET plain_size = sqlc.arg(plain_size),
    stored_size = sqlc.arg(stored_size)
WHERE file_id = sqlc.arg(file_id)
  AND part_no = sqlc.arg(part_no)
  AND (plain_size IS NULL OR stored_size IS NULL);

-- name: UpdateFilePartSizesMany :execrows
UPDATE /* TEMPLATE: schema */file_parts AS part
SET plain_size = input.plain_size,
    stored_size = input.stored_size
FROM jsonb_to_recordset(sqlc.arg(parts)::jsonb) AS input(
    part_no integer, plain_size bigint, stored_size bigint
)
WHERE part.file_id = sqlc.arg(file_id)
  AND part.part_no = input.part_no
  AND (part.plain_size IS NULL OR part.stored_size IS NULL);
