-- +goose Up
-- +goose StatementBegin

CREATE OR REPLACE PROCEDURE teldrive.update_size() LANGUAGE PLPGSQL AS $$
DECLARE
    rec RECORD;
    total_size BIGINT;
BEGIN
    FOR rec IN
        SELECT id
        FROM teldrive.files
        WHERE type = 'folder'
        ORDER BY depth DESC
    LOOP
        total_size := (
            SELECT SUM(size) AS total_size
            FROM teldrive.files
            WHERE parent_id = rec.id
        );

        UPDATE teldrive.files
        SET size = total_size
        WHERE id = rec.id;
    END LOOP;
END;
$$;


CREATE OR REPLACE PROCEDURE teldrive.delete_files(
  IN file_ids TEXT[],
  IN op TEXT DEFAULT 'bulk'
) LANGUAGE plpgsql
AS $$
DECLARE
  rec RECORD;
BEGIN
  IF op = 'bulk' THEN
      FOR rec IN
          SELECT
              id,
              type
          FROM
              teldrive.files
          WHERE
              id = ANY (file_ids)
      LOOP
          IF rec.type = 'folder' THEN
              CALL teldrive.delete_files(ARRAY[rec.id], 'single');
              DELETE FROM teldrive.files WHERE id = rec.id;
          ELSE
             UPDATE teldrive.files SET status = 'pending_deletion' WHERE id = rec.id;

          END IF;
      END LOOP;
  ELSE
      FOR rec IN
          SELECT
              id,
              type
          FROM
              teldrive.files
          WHERE
              parent_id = file_ids[1]
      LOOP
          IF rec.type = 'folder' THEN
              CALL teldrive.delete_files(ARRAY[rec.id], 'single');
              DELETE FROM teldrive.files WHERE id = rec.id;
          ELSE
             UPDATE teldrive.files SET status = 'pending_deletion' WHERE id = rec.id;  
          END IF;
      END LOOP;
  END IF;
END;
$$;


CREATE OR REPLACE FUNCTION teldrive.create_directories(
    IN u_id BIGINT,
    IN long_path TEXT
) RETURNS SETOF teldrive.files AS $$
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

    SELECT id INTO current_directory_id
    FROM teldrive.files
    WHERE parent_id = 'root' AND user_id = u_id;

    FOR directory_name IN SELECT unnest(path_parts) LOOP
        path_so_far := CONCAT(path_so_far, '/', directory_name);
        depth_dir := depth_dir + 1;

        SELECT id INTO new_directory_id
        FROM teldrive.files
        WHERE parent_id = current_directory_id
          AND "name" = directory_name
          AND "user_id" = u_id;

        IF new_directory_id IS NULL THEN
            INSERT INTO teldrive.files ("name", "type", mime_type, parent_id, "user_id", starred, "depth", "path")
            VALUES (directory_name, 'folder', 'drive/folder', current_directory_id, u_id, false, depth_dir, path_so_far)
            RETURNING id INTO new_directory_id;
        END IF;

        current_directory_id := new_directory_id;
    END LOOP;

    RETURN QUERY SELECT * FROM teldrive.files WHERE id = current_directory_id;
END;
$$ LANGUAGE plpgsql;



CREATE OR REPLACE FUNCTION teldrive.split_path(path text, OUT parent text, OUT base text) AS $$
BEGIN
    IF path = '/' THEN
        parent := '/';
        base := NULL;
        RETURN;
    END IF;

    IF left(path, 1) <> '/' THEN
        path := '/' || path;
    END IF;

    IF right(path, 1) = '/' THEN
        path := left(path, length(path) - 1);
    END IF;

    parent := left(path, length(path) - position('/' in reverse(path)));
    base := right(path, position('/' in reverse(path)) - 1);
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION teldrive.update_folder(
  folder_id TEXT,
  new_name TEXT,
  new_path TEXT DEFAULT NULL
) RETURNS SETOF teldrive.files
LANGUAGE plpgsql
AS $$
DECLARE
  folder RECORD;
  path_items TEXT[];
BEGIN
  IF new_path IS NULL THEN
      SELECT
          *
      INTO
          folder
      FROM
          teldrive.files
      WHERE
          id = folder_id;
      
      path_items := string_to_array(folder.path, '/');
      
      path_items[array_length(path_items, 1)] := new_name;
      
      new_path := array_to_string(path_items, '/');
  END IF;
  
  UPDATE
      teldrive.files
  SET
      path = new_path,
      name = new_name
  WHERE
      id = folder_id;
  
  FOR folder IN
      SELECT
          *
      FROM
          teldrive.files
      WHERE
          type = 'folder'
          AND parent_id = folder_id
  LOOP
     perform from teldrive.update_folder(
          folder.id,
          folder.name,
          concat(new_path, '/', folder.name)
      );
  END LOOP;
 
  RETURN QUERY
  SELECT
      *
  FROM
      teldrive.files
  WHERE
      id = folder_id;
END;
$$;

CREATE OR REPLACE FUNCTION teldrive.move_directory(src text, dest text,u_id bigint) RETURNS VOID AS $$
DECLARE
    src_parent TEXT;
    src_base TEXT;
    dest_parent TEXT;
    dest_base TEXT;
    dest_id text;
    src_id text;
BEGIN
	
    IF NOT EXISTS (SELECT 1 FROM teldrive.files WHERE path = src and user_id = u_id) THEN
        RAISE EXCEPTION 'source directory not found';
    END IF;
   
    IF EXISTS (SELECT 1 FROM teldrive.files WHERE path = dest and user_id = u_id) THEN
        RAISE EXCEPTION 'destination directory exists';
    END IF;
   
    SELECT parent, base INTO src_parent,src_base FROM teldrive.split_path(src);
   
    SELECT parent, base INTO dest_parent, dest_base FROM teldrive.split_path(dest);
   
    IF src_parent != dest_parent then
      select id into dest_id from teldrive.create_directories(u_id,dest);
      update teldrive.files set parent_id = dest_id where parent_id = (select id from teldrive.files where path = src) and id != dest_id and user_id = u_id;
      
      IF POSITION(CONCAT(src,'/') IN dest) = 0 then
         delete from teldrive.files where path = src and user_id = u_id;
      END IF;
     
    END IF;

    IF src_base != dest_base and src_parent = dest_parent then
       select id into src_id from teldrive.files where path = src and user_id = u_id;
       perform from teldrive.update_folder(src_id,dest_base);
    END IF;

END;
$$ LANGUAGE plpgsql;



CREATE OR REPLACE FUNCTION teldrive.account_stats(
    IN u_id BIGINT
)  RETURNS TABLE (total_size BIGINT, total_files BIGINT, channel_id BIGINT, channel_name TEXT ) AS $$
DECLARE
    total_size BIGINT;
    total_files BIGINT;
    channel_id BIGINT;
    channel_name TEXT;
BEGIN
    SELECT COUNT(*), coalesce(SUM(size),0) into total_files,total_size FROM teldrive.files WHERE user_id=u_id AND type= 'file' and status='active';
    SELECT c.channel_id ,c.channel_name into channel_id, channel_name FROM teldrive.channels c  WHERE selected=TRUE AND user_id=u_id;
    RETURN QUERY SELECT total_size,total_files,channel_id,channel_name;
END;
$$ LANGUAGE plpgsql;



-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP PROCEDURE IF EXISTS teldrive.update_size;
DROP PROCEDURE IF EXISTS teldrive.delete_files;
DROP FUNCTION IF EXISTS teldrive.create_directories;
DROP FUNCTION IF EXISTS teldrive.split_path;
DROP FUNCTION IF EXISTS teldrive.update_folder;
DROP FUNCTION IF EXISTS teldrive.move_directory;
DROP FUNCTION IF EXISTS teldrive.account_stats;
-- +goose StatementEnd
