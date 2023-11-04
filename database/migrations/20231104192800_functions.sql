-- +goose Up
-- +goose StatementBegin

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

drop function if exists teldrive.move_directory;

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

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
drop function if exists teldrive.split_path;
drop function if exists teldrive.update_folder;
drop function if exists teldrive.move_directory;
-- +goose StatementEnd
