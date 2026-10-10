-- +goose Up
-- +goose StatementBegin
DROP FUNCTION IF EXISTS teldrive.account_stats;
DROP FUNCTION IF EXISTS teldrive.update_folder;

CREATE OR REPLACE FUNCTION teldrive.create_directories(u_id bigint, long_path text)
 RETURNS SETOF teldrive.files
 LANGUAGE plpgsql
AS $function$
DECLARE
    path_parts TEXT[];
    current_directory_id TEXT;
    new_directory_id TEXT;
    directory_name TEXT;
    path_so_far TEXT;
BEGIN
    path_parts := string_to_array(regexp_replace(long_path, '^/+', ''), '/');

    path_so_far := '';

    SELECT id INTO current_directory_id
    FROM teldrive.files
    WHERE parent_id = 'root' AND user_id = u_id;

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

-- +goose StatementEnd