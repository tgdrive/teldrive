-- +goose Up
-- +goose StatementBegin

DROP FUNCTION IF EXISTS teldrive.update_folder;
DROP FUNCTION IF EXISTS teldrive.move_directory;
DROP FUNCTION IF EXISTS teldrive.move_items;


CREATE OR REPLACE FUNCTION teldrive.update_folder(
  folder_id TEXT,
  new_name TEXT,
  u_id bigint
) RETURNS SETOF teldrive.files
LANGUAGE plpgsql
AS $$
DECLARE
  old_path TEXT;
  new_path TEXT;
BEGIN

  SELECT
      path
  INTO
      old_path
  FROM
      teldrive.files
  WHERE
      id = folder_id and user_id = u_id;

  new_path := substring(old_path from '(.*)/[^/]*$') || '/' || new_name;
 
  UPDATE
     teldrive.files
  SET path = new_path ,name = new_name WHERE id = folder_id and user_id = u_id;

  UPDATE
      teldrive.files
  SET path = new_path || substring(path, length(old_path) + 1)
  WHERE
       type='folder' and  user_id = u_id and path LIKE old_path || '/%';
  
  RETURN QUERY
  SELECT
      *
  FROM
      teldrive.files
  WHERE
      id = folder_id;
END;
$$;


CREATE OR REPLACE FUNCTION teldrive.move_directory(src text, dest text, u_id bigint)
 RETURNS void
 LANGUAGE plpgsql
AS $$
DECLARE
    src_parent TEXT;
    src_base TEXT;
    dest_parent TEXT;
    dest_base TEXT;
    dest_id text;
    dest_parent_id text;
    src_id text;
begin
	
	SELECT id into src_id FROM teldrive.files WHERE path = src and user_id = u_id;

	SELECT id into dest_id FROM teldrive.files WHERE path = dest and user_id = u_id;
	
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
     update
     teldrive.files
     SET path = dest || substring(path, length(src) + 1)
     where type='folder' and  user_id = u_id and path LIKE src || '/%';
     delete from teldrive.files where id = src_id;
   
    END IF;
    
    IF src_base != dest_base and src_parent = dest_parent then
       perform teldrive.update_folder(src_id,dest_base,u_id);
    END IF;

END;
$$
;

CREATE OR REPLACE FUNCTION teldrive.move_items(file_ids text[], dest text, u_id bigint)
RETURNS VOID AS $$
declare
dest_id TEXT;
BEGIN

    SELECT id INTO dest_id FROM teldrive.files WHERE path = dest and user_id = u_id;

    IF dest_id is NULL then
    select id into dest_id from teldrive.create_directories(u_id,dest);
    END IF;

    UPDATE teldrive.files
    SET parent_id = dest_id
    WHERE id = ANY(file_ids);

    WITH RECURSIVE folders AS (
        SELECT id, name, path, 
        CASE 
            WHEN dest = '/' THEN '/' || name
            ELSE dest || '/' || name
        END as new_path
        FROM teldrive.files
        WHERE id = ANY(file_ids) AND type = 'folder' and user_id = u_id
        UNION ALL
        SELECT f.id, f.name, f.path, 
        CASE 
            WHEN fo.new_path = '/' THEN '/' || f.name
            ELSE fo.new_path || '/' || f.name
        END
        FROM teldrive.files f
        INNER JOIN folders fo ON f.parent_id = fo.id WHERE type = 'folder'
    )
    UPDATE teldrive.files
    SET path = folders.new_path
    FROM folders
    WHERE teldrive.files.id = folders.id;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd