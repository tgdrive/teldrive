-- +goose Up
-- +goose StatementBegin

DROP FUNCTION IF EXISTS teldrive.get_file_from_path;

CREATE OR REPLACE FUNCTION teldrive.get_file_from_path(full_path text, u_id bigint,throw_error boolean DEFAULT false)
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
   
    IF throw_error IS true AND target_id IS NULL THEN
        RAISE EXCEPTION 'file not found for path: %', full_path;
    END IF;
   
    RETURN QUERY select * from teldrive.files where id=target_id;

END;
$function$
;


-- +goose StatementEnd