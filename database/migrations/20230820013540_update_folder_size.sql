-- +goose Up
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION teldrive.update_size_function()
RETURNS TRIGGER AS $$
DECLARE
    rec RECORD;
    total_size BIGINT;
    batch_size INT := 100;
    lower_bound INT := 0;
    upper_bound INT := batch_size;
BEGIN
    LOOP
        FOR rec IN
            SELECT id, size
            FROM teldrive.files
            WHERE type = 'folder'
            ORDER BY depth DESC
            OFFSET lower_bound
            LIMIT batch_size
        LOOP
            SELECT COALESCE(SUM(size),0) INTO total_size
            FROM teldrive.files
            WHERE parent_id = rec.id
            AND status != 'pending_deletion';
            
            IF total_size <> rec.size THEN
                UPDATE teldrive.files
                SET size = total_size
                WHERE id = rec.id;
            END IF;

            EXIT WHEN NOT FOUND;
        END LOOP;

        lower_bound := upper_bound;
        upper_bound := upper_bound + batch_size;

        EXIT WHEN NOT FOUND;
    END LOOP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER update_folder_size_after_insert_update_delete
AFTER INSERT OR UPDATE OR DELETE ON teldrive.files
FOR EACH ROW
EXECUTE PROCEDURE teldrive.update_size_function();
-- +goose StatementEnd


-- +goose Down
-- +goose StatementBegin
DROP TRIGGER IF EXISTS update_folder_size_after_insert_update_delete ON teldrive.files;
DROP FUNCTION IF EXISTS teldrive.update_size_function;
-- +goose StatementEnd
