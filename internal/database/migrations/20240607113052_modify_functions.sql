-- +goose Up
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION teldrive.get_file_from_path(full_path text,u_id bigint)
RETURNS setof teldrive.files  AS $$
DECLARE
    target_id text;
begin
	
    IF full_path = '/' then
       RETURN QUERY select * from teldrive.files as root where root.parent_id = 'root';
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
            root.parent_id = 'root' AND root.user_id = u_id
        
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
   
    RETURN QUERY select * from teldrive.files where id=target_id;

END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION teldrive.create_directories(u_id bigint, long_path text)
 RETURNS SETOF teldrive.files AS $$
DECLARE
    path_parts TEXT[];
    current_directory_id TEXT;
    new_directory_id TEXT;
    directory_name TEXT;
    path_so_far TEXT;
    depth_dir INTEGER;
BEGIN
    path_parts := string_to_array(regexp_replace(long_path, '^/+', ''), '/');
    path_so_far := '';
    depth_dir := 0;

    SELECT id INTO current_directory_id FROM teldrive.files WHERE parent_id = 'root';

    FOR directory_name IN SELECT unnest(path_parts) LOOP
        depth_dir := depth_dir + 1;

        SELECT id INTO new_directory_id
        FROM teldrive.files
        WHERE parent_id = current_directory_id
        AND "name" = directory_name
        AND "user_id" = u_id;

        IF new_directory_id IS NULL THEN
            INSERT INTO teldrive.files ("name", "type", mime_type, parent_id, "user_id", starred, "depth")
            VALUES (directory_name, 'folder', 'drive/folder', current_directory_id, u_id, false, depth_dir)
            RETURNING id INTO new_directory_id;
        END IF;

        current_directory_id := new_directory_id;
    END LOOP;

    RETURN QUERY SELECT * FROM teldrive.files WHERE id = current_directory_id;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION teldrive.move_directory(src text, dest text, u_id bigint)
 RETURNS void AS $$
DECLARE
    src_parent TEXT;
    src_base TEXT;
    dest_parent TEXT;
    dest_base TEXT;
    dest_id text;
    dest_parent_id text;
    src_id text;
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
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION teldrive.move_items(file_ids text[], dest text, u_id bigint)
 RETURNS void AS $$
declare
dest_id TEXT;
BEGIN

    select id into dest_id from teldrive.get_file_from_path(dest,u_id);
    
    IF dest_id is NULL then
    select id into dest_id from teldrive.create_directories(u_id,dest);
    END IF;

    UPDATE teldrive.files
    SET parent_id = dest_id
    WHERE id = ANY(file_ids);
END;
$$ LANGUAGE plpgsql;

DROP INDEX IF EXISTS "path_idx";
ALTER TABLE "teldrive"."files" DROP COLUMN IF EXISTS "path";
-- +goose StatementEnd
