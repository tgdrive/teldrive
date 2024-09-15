-- +goose Up
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION teldrive.get_path_from_file_id(file_id uuid)
 RETURNS text
 LANGUAGE plpgsql
AS $function$
DECLARE
    full_path TEXT;
    trimmed_path TEXT;
BEGIN
    WITH RECURSIVE path_hierarchy AS (
        SELECT
            f.id,
            f.name,
            f.parent_id,
            f.name AS path_segment
        FROM
            teldrive.files f
        WHERE
            f.id = file_id
    
        UNION ALL
    
        SELECT
            p.id,
            p.name,
            p.parent_id,
            CASE 
                WHEN ph.parent_id IS NULL THEN ph.path_segment
                ELSE p.name || '/' || ph.path_segment
            END AS path_segment
        FROM
            teldrive.files p
        JOIN
            path_hierarchy ph ON ph.parent_id = p.id
    )
    
    SELECT path_segment INTO full_path
    FROM path_hierarchy
    WHERE parent_id IS NULL;
    
    SELECT 
        CASE 
            WHEN position('/' in full_path) > 0 THEN substring(full_path from position('/' in full_path) + 1)
            ELSE full_path
        END INTO trimmed_path;
    
    RETURN '/' || trimmed_path;
END;
$function$
;
-- +goose StatementEnd