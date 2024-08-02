-- +goose Up
-- +goose StatementBegin

UPDATE teldrive.files
SET parent_id = NULL where files.parent_id = 'root';

ALTER TABLE teldrive.files
ADD COLUMN uuid_id UUID,
ADD COLUMN uuid_parent_id UUID;

UPDATE teldrive.files
SET uuid_id = uuid7(files.created_at);

UPDATE teldrive.files f1
SET uuid_parent_id = f2.uuid_id
FROM teldrive.files f2
WHERE f1.parent_id = f2.id;

ALTER TABLE teldrive.files
DROP CONSTRAINT IF EXISTS files_pkey;

ALTER TABLE teldrive.files
RENAME COLUMN id TO old_id;

ALTER TABLE teldrive.files
RENAME COLUMN uuid_id TO id;

ALTER TABLE teldrive.files
RENAME COLUMN parent_id TO old_parent_id;

ALTER TABLE teldrive.files
RENAME COLUMN uuid_parent_id TO parent_id;

ALTER TABLE teldrive.files
ADD CONSTRAINT files_pkey PRIMARY KEY (id);

ALTER TABLE teldrive.files
ALTER COLUMN id SET DEFAULT uuid7();

DROP INDEX IF EXISTS teldrive.files_category_type_user_id_index;
DROP INDEX IF EXISTS teldrive.name_idx;
DROP INDEX IF EXISTS teldrive.name_search_idx;
DROP INDEX IF EXISTS teldrive.name_status_user_id_idx;
DROP INDEX IF EXISTS teldrive.parent_idx;
DROP INDEX IF EXISTS teldrive.starred_updated_at_idx;
DROP INDEX IF EXISTS teldrive.status_idx;
DROP INDEX IF EXISTS teldrive.status_user_id_idx;
DROP INDEX IF EXISTS teldrive.unique_file;
DROP INDEX IF EXISTS teldrive.unique_folder;
DROP INDEX IF EXISTS teldrive.updated_at_idx;
DROP INDEX IF EXISTS teldrive.updated_at_status_user_id_idx;
DROP INDEX IF EXISTS teldrive.user_id_idx;
DROP PROCEDURE IF EXISTS teldrive.update_size;
DROP FUNCTION IF EXISTS teldrive.move_items;
DROP FUNCTION IF EXISTS teldrive.move_directory;
DROP PROCEDURE IF EXISTS teldrive.delete_folder_recursive;
DROP PROCEDURE IF EXISTS teldrive.delete_files_bulk;
DROP FUNCTION IF EXISTS teldrive.create_directories;
DROP FUNCTION IF EXISTS teldrive.get_file_from_path;


CREATE INDEX idx_files_category_type_user_id ON teldrive.files USING btree (category, type, user_id);
CREATE INDEX idx_files_name ON teldrive.files USING btree (name);
CREATE INDEX idx_files_name_search ON teldrive.files USING pgroonga (regexp_replace(name, '[.,-_]'::text, ' '::text, 'g'::text)) WITH (tokenizer='TokenNgram');
CREATE INDEX idx_files_name_user_id_status ON teldrive.files USING btree (name, user_id, status);
CREATE INDEX idx_files_parent_id ON teldrive.files USING btree (parent_id);
CREATE INDEX idx_files_starred_updated_at ON teldrive.files USING btree (starred, updated_at DESC);
CREATE INDEX idx_files_status ON teldrive.files USING btree (status);
CREATE INDEX idx_files_updated_at_user_id_status ON teldrive.files USING btree (updated_at DESC, user_id, status);
CREATE UNIQUE INDEX idx_files_unique_file ON teldrive.files USING btree (name, parent_id, user_id, size) WHERE ((status = 'active'::text) AND (type = 'file'::text));
CREATE UNIQUE INDEX idx_files_unique_folder ON teldrive.files USING btree (name, parent_id, user_id) WHERE (type = 'folder'::text);
CREATE INDEX idx_files_updated_at ON teldrive.files USING btree (updated_at);
CREATE INDEX idx_files_user_id ON teldrive.files USING btree (user_id);


CREATE OR REPLACE FUNCTION teldrive.create_directories(u_id bigint, long_path text)
 RETURNS SETOF teldrive.files
 LANGUAGE plpgsql
AS $function$
DECLARE
    path_parts TEXT[];
    current_directory_id UUID;
    new_directory_id UUID;
    directory_name TEXT;
    path_so_far TEXT;
BEGIN
    path_parts := string_to_array(regexp_replace(long_path, '^/+', ''), '/');

    path_so_far := '';

    SELECT id INTO current_directory_id
    FROM teldrive.files
    WHERE parent_id is NULL AND user_id = u_id AND type = 'folder';

    FOR directory_name IN SELECT unnest(path_parts) LOOP
        path_so_far := CONCAT(path_so_far, '/', directory_name);

        SELECT id INTO new_directory_id
        FROM teldrive.files
        WHERE parent_id = current_directory_id
          AND "name" = directory_name
          AND "user_id" = u_id;

        IF new_directory_id IS NULL THEN
            INSERT INTO teldrive.files ("name", "type", mime_type, parent_id, "user_id", starred)
            VALUES (directory_name, 'folder', 'drive/folder', current_directory_id, u_id, false)
            RETURNING id INTO new_directory_id;
        END IF;

        current_directory_id := new_directory_id;
    END LOOP;

    RETURN QUERY SELECT * FROM teldrive.files WHERE id = current_directory_id;
END;
$function$
;

CREATE OR REPLACE FUNCTION teldrive.get_file_from_path(full_path text, u_id bigint, throw_error boolean DEFAULT false)
 RETURNS SETOF teldrive.files
 LANGUAGE plpgsql
AS $function$
DECLARE
    target_id UUID;
begin
    
    IF full_path = '/' then
      full_path := '';
    END IF;
   
    WITH RECURSIVE dir_hierarchy AS (
        SELECT
            root.id,
            root.name,
            root.parent_id,
            0 AS depth,
            '' as path
        FROM
            teldrive.files as root
        WHERE
            root.parent_id is NULL AND root.user_id = u_id and root.type='folder'
        
        UNION ALL
        
        SELECT
            f.id,
            f.name,
            f.parent_id,
            dh.depth + 1 AS depth,
            dh.path || '/' || f.name
        FROM
            teldrive.files f
        JOIN
            dir_hierarchy dh ON dh.id = f.parent_id
        WHERE f.type = 'folder' AND f.user_id = u_id
    )

    SELECT id into target_id FROM dir_hierarchy dh
    WHERE dh.path = full_path
    ORDER BY dh.depth DESC
    LIMIT 1;
   
    IF throw_error IS true AND target_id IS NULL THEN
        RAISE EXCEPTION 'file not found for path: %', full_path;
    END IF;
   
    RETURN QUERY select * from teldrive.files where id=target_id;

END;
$function$
;

CREATE OR REPLACE PROCEDURE teldrive.delete_files_bulk(IN file_ids text[], IN u_id bigint)
 LANGUAGE plpgsql
AS $procedure$
DECLARE
    folder_ids UUID[];
BEGIN
    WITH RECURSIVE folder_tree AS (
        SELECT id
        FROM teldrive.files
        WHERE id = ANY (file_ids::UUID[]) 
        AND user_id = u_id AND type = 'folder'
        
        UNION ALL

        SELECT f.id
        FROM teldrive.files f
        JOIN folder_tree ft ON f.parent_id = ft.id
        WHERE f.user_id = u_id AND f.type = 'folder'
    ) SELECT array_agg(id) INTO folder_ids FROM folder_tree;
    
    UPDATE teldrive.files
    SET status = 'pending_deletion'
    WHERE (id = ANY (file_ids::UUID[]) OR parent_id = ANY (folder_ids))
    AND type = 'file' AND user_id = u_id;

    DELETE FROM teldrive.files
    WHERE id = ANY (folder_ids) AND user_id = u_id;
END;
$procedure$
;


CREATE OR REPLACE PROCEDURE teldrive.delete_folder_recursive(IN src text, IN u_id bigint)
 LANGUAGE plpgsql
AS $procedure$
DECLARE
    folder_id UUID;
    folder_ids UUID[];
BEGIN

    IF position('/' in src) = 1 THEN
        select id into folder_id from teldrive.get_file_from_path(src,u_id);
        IF folder_id IS NULL THEN
            RAISE EXCEPTION 'source not found';
        END IF;
    ELSE
        folder_id := src;
        IF NOT EXISTS (SELECT 1 FROM teldrive.files WHERE id = folder_id) THEN
            RAISE EXCEPTION 'source not found';
        END IF;
    END IF;

    WITH RECURSIVE folder_tree AS (
        SELECT id
        FROM teldrive.files
        WHERE id = folder_id 
        AND user_id = u_id and type='folder'
        
        UNION ALL
        
        SELECT f.id
        FROM teldrive.files f
        JOIN folder_tree ft ON f.parent_id = ft.id 
        WHERE f.user_id = u_id and f.type='folder' 
    ) SELECT array_agg(id) INTO folder_ids FROM folder_tree;
    
    UPDATE teldrive.files
    SET status = 'pending_deletion'
    WHERE parent_id = ANY (folder_ids) and type='file' and user_id = u_id;
   
    DELETE FROM teldrive.files
    WHERE id = ANY (folder_ids) AND user_id = u_id;
END;
$procedure$
;


CREATE OR REPLACE FUNCTION teldrive.move_directory(src text, dest text, u_id bigint)
 RETURNS void
 LANGUAGE plpgsql
AS $function$
DECLARE
    src_parent TEXT;
    src_base TEXT;
    dest_parent TEXT;
    dest_base TEXT;
    dest_id UUID;
    dest_parent_id UUID;
    src_id UUID;
begin

    select id into src_id from teldrive.get_file_from_path(src,u_id);

    select id into dest_id from teldrive.get_file_from_path(dest,u_id);
    
    IF src_id is NULL THEN
        RAISE EXCEPTION 'source directory not found';
    END IF;
   
    IF dest_id is not NULL then
       RAISE EXCEPTION 'destination directory exists';
    END IF;
   
    SELECT parent, base INTO src_parent,src_base FROM teldrive.split_path(src);
   
    SELECT parent, base INTO dest_parent, dest_base FROM teldrive.split_path(dest);
   
    IF src_parent != dest_parent then
     select id into dest_id from teldrive.create_directories(u_id,dest);
     UPDATE teldrive.files SET parent_id = dest_id WHERE parent_id = src_id;
     delete from teldrive.files where id = src_id;
    END IF;
    
    IF src_base != dest_base and src_parent = dest_parent then
      UPDATE teldrive.files SET name = dest_base WHERE id = src_id;
    END IF;

END;
$function$
;


CREATE OR REPLACE FUNCTION teldrive.move_items(file_ids text[], dest text, u_id bigint)
 RETURNS void
 LANGUAGE plpgsql
AS $function$
declare
dest_id UUID;
BEGIN

    select id into dest_id from teldrive.get_file_from_path(dest,u_id);
    
    IF dest_id is NULL then
    select id into dest_id from teldrive.create_directories(u_id,dest);
    END IF;

    UPDATE teldrive.files
    SET parent_id = dest_id
    WHERE id = ANY(file_ids::UUID[]);
END;
$function$
;

CREATE OR REPLACE PROCEDURE teldrive.update_size()
 LANGUAGE plpgsql
AS $procedure$
begin
    
    WITH RECURSIVE folder_hierarchy AS (
    
        SELECT
            id,
            name,
            parent_id,
            ARRAY[id] AS path
        FROM
            teldrive.files
        WHERE
            type = 'folder' AND parent_id is NULL

        UNION ALL

        SELECT
            f.id,
            f.name,
            f.parent_id,
            fh.path || f.id
        FROM
            teldrive.files f
        JOIN
            folder_hierarchy fh ON f.parent_id = fh.id
        WHERE
            f.type = 'folder'
    )
    , folder_sizes AS (
        SELECT
            f.id,
            f.path,
            COALESCE(SUM(CASE WHEN c.type != 'folder' THEN c.size ELSE 0 END), 0) AS direct_size
        FROM
            folder_hierarchy f
        LEFT JOIN
            teldrive.files c ON f.id = c.parent_id
        GROUP BY
            f.id, f.path
    )
    , cumulative_sizes AS (
        SELECT
            fs.id,
            SUM(fs2.direct_size) AS total_size
        FROM
            folder_sizes fs
        JOIN
            folder_sizes fs2 ON fs2.path @> fs.path
        GROUP BY
            fs.id
    )
    UPDATE teldrive.files f
    SET size = cs.total_size
    FROM cumulative_sizes cs
    WHERE f.id = cs.id AND f.type = 'folder';
END;
$procedure$
;

ALTER TABLE teldrive.files
DROP COLUMN old_id,
DROP COLUMN old_parent_id;
DROP FUNCTION IF EXISTS teldrive.generate_uid;
-- +goose StatementEnd