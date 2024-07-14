-- +goose Up
-- +goose StatementBegin
DROP INDEX IF EXISTS teldrive.name_numeric_idx;
DROP INDEX IF EXISTS teldrive.parent_name_numeric_idx;
DROP INDEX IF EXISTS teldrive.path_idx;
DROP COLLATION IF EXISTS numeric;
CREATE INDEX IF NOT EXISTS name_idx ON teldrive.files (name);
CREATE INDEX IF NOT EXISTS updated_at_status_user_id_idx ON teldrive.files (updated_at DESC, user_id, status);
CREATE INDEX IF NOT EXISTS name_status_user_id_idx ON teldrive.files (name, user_id, status);
CREATE INDEX IF NOT EXISTS updated_at_idx ON teldrive.files (updated_at);

CREATE OR REPLACE FUNCTION teldrive.get_file_from_path(full_path text, u_id bigint)
 RETURNS SETOF teldrive.files
 LANGUAGE plpgsql
AS $function$
DECLARE
    target_id text;
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
   
    IF target_id IS NULL THEN
        RAISE EXCEPTION 'file not found for path: %', full_path;
    END IF;
   
    RETURN QUERY select * from teldrive.files where id=target_id;

END;
$function$
;

-- +goose StatementEnd
