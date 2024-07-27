-- +goose Up
-- +goose StatementBegin
ALTER TABLE teldrive.files DROP COLUMN depth;

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
            type = 'folder' AND parent_id = 'root'

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
$procedure$;
-- +goose StatementEnd