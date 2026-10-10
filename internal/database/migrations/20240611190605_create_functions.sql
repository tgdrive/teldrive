-- +goose Up
-- +goose StatementBegin
DROP PROCEDURE IF EXISTS teldrive.delete_files;

CREATE OR REPLACE PROCEDURE teldrive.delete_folder_recursive(
    IN src text,
    IN u_id bigint
)
LANGUAGE plpgsql
AS $procedure$
DECLARE
    folder_id text;
    folder_ids text[];
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
$procedure$;

CREATE OR REPLACE PROCEDURE teldrive.delete_files_bulk(
    IN file_ids text[],
    IN u_id bigint
)
LANGUAGE plpgsql
AS $procedure$
DECLARE
    folder_ids text[];
BEGIN
    WITH RECURSIVE folder_tree AS (
        SELECT id
        FROM teldrive.files
        WHERE id = ANY (file_ids) 
        AND user_id = u_id AND type = 'folder'
        
        UNION ALL

        SELECT f.id
        FROM teldrive.files f
        JOIN folder_tree ft ON f.parent_id = ft.id
        WHERE f.user_id = u_id AND f.type = 'folder'
    ) SELECT array_agg(id) INTO folder_ids FROM folder_tree;
    
    UPDATE teldrive.files
    SET status = 'pending_deletion'
    WHERE (id = ANY (file_ids) OR parent_id = ANY (folder_ids))
    AND type = 'file' AND user_id = u_id;

    DELETE FROM teldrive.files
    WHERE id = ANY (folder_ids) AND user_id = u_id;
END;
$procedure$
-- +goose StatementEnd