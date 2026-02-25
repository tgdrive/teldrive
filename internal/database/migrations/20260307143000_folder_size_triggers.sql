-- +goose Up
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION teldrive.apply_folder_size_delta(start_folder_id UUID, delta_size BIGINT)
RETURNS VOID
LANGUAGE plpgsql
AS $$
BEGIN
    IF start_folder_id IS NULL OR delta_size = 0 THEN
        RETURN;
    END IF;

    WITH RECURSIVE ancestors AS (
        SELECT f.id, f.parent_id
        FROM teldrive.files f
        WHERE f.id = start_folder_id
          AND f.type = 'folder'

        UNION ALL

        SELECT p.id, p.parent_id
        FROM teldrive.files p
        JOIN ancestors a ON p.id = a.parent_id
        WHERE p.type = 'folder'
    )
    UPDATE teldrive.files f
    SET size = COALESCE(f.size, 0) + delta_size
    FROM ancestors a
    WHERE f.id = a.id;
END;
$$;

CREATE OR REPLACE FUNCTION teldrive.files_folder_size_trigger()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
    old_effective_size BIGINT := 0;
    new_effective_size BIGINT := 0;
    moved_folder_size  BIGINT := 0;
BEGIN
    IF TG_OP = 'INSERT' THEN
        IF NEW.type = 'file' AND NEW.status = 'active' THEN
            PERFORM teldrive.apply_folder_size_delta(NEW.parent_id, COALESCE(NEW.size, 0));
        END IF;
        RETURN NEW;
    END IF;

    IF TG_OP = 'DELETE' THEN
        IF OLD.type = 'file' AND OLD.status = 'active' THEN
            PERFORM teldrive.apply_folder_size_delta(OLD.parent_id, -COALESCE(OLD.size, 0));
        END IF;
        RETURN OLD;
    END IF;

    -- UPDATE path
    IF OLD.type = 'file' OR NEW.type = 'file' THEN
        old_effective_size := CASE
            WHEN OLD.type = 'file' AND OLD.status = 'active' THEN COALESCE(OLD.size, 0)
            ELSE 0
        END;

        new_effective_size := CASE
            WHEN NEW.type = 'file' AND NEW.status = 'active' THEN COALESCE(NEW.size, 0)
            ELSE 0
        END;

        IF OLD.parent_id IS NOT DISTINCT FROM NEW.parent_id
           AND OLD.user_id IS NOT DISTINCT FROM NEW.user_id THEN
            PERFORM teldrive.apply_folder_size_delta(NEW.parent_id, new_effective_size - old_effective_size);
        ELSE
            PERFORM teldrive.apply_folder_size_delta(OLD.parent_id, -old_effective_size);
            PERFORM teldrive.apply_folder_size_delta(NEW.parent_id, new_effective_size);
        END IF;

        RETURN NEW;
    END IF;

    -- Folder move: shift already-computed subtree size between ancestor chains.
    IF OLD.type = 'folder' AND NEW.type = 'folder'
       AND (OLD.parent_id IS DISTINCT FROM NEW.parent_id
            OR OLD.user_id IS DISTINCT FROM NEW.user_id) THEN
        moved_folder_size := COALESCE(NEW.size, 0);
        PERFORM teldrive.apply_folder_size_delta(OLD.parent_id, -moved_folder_size);
        PERFORM teldrive.apply_folder_size_delta(NEW.parent_id, moved_folder_size);
    END IF;

    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS trg_files_folder_size ON teldrive.files;

CREATE TRIGGER trg_files_folder_size
AFTER INSERT OR DELETE OR UPDATE OF parent_id, size, status, type, user_id
ON teldrive.files
FOR EACH ROW
EXECUTE FUNCTION teldrive.files_folder_size_trigger();
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER IF EXISTS trg_files_folder_size ON teldrive.files;
DROP FUNCTION IF EXISTS teldrive.files_folder_size_trigger();
DROP FUNCTION IF EXISTS teldrive.apply_folder_size_delta(UUID, BIGINT);
