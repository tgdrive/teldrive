-- +goose Up
-- Replace row-level folder size trigger with statement-level transition-table triggers.
-- +goose StatementBegin
DROP TRIGGER IF EXISTS trg_files_folder_size ON teldrive.files;
DROP FUNCTION IF EXISTS teldrive.files_folder_size_trigger();
DROP FUNCTION IF EXISTS teldrive.apply_folder_size_delta(UUID, BIGINT);

CREATE OR REPLACE FUNCTION teldrive.files_folder_size_insert_stmt_trigger()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    IF pg_trigger_depth() > 1 THEN
        RETURN NULL;
    END IF;

    WITH RECURSIVE changed AS (
        SELECT n.parent_id AS start_folder_id, SUM(COALESCE(n.size, 0)) AS delta
        FROM new_rows n
        WHERE n.type = 'file'
          AND n.status = 'active'
          AND n.parent_id IS NOT NULL
        GROUP BY n.parent_id
    ),
    recursive_ancestors AS (
        SELECT f.id AS folder_id, f.parent_id, c.delta
        FROM changed c
        JOIN teldrive.files f ON f.id = c.start_folder_id
        WHERE f.type = 'folder'

        UNION ALL

        SELECT p.id AS folder_id, p.parent_id, ra.delta
        FROM recursive_ancestors ra
        JOIN teldrive.files p ON p.id = ra.parent_id
        WHERE p.type = 'folder'
    ),
    deltas AS (
        SELECT folder_id, SUM(delta) AS delta
        FROM recursive_ancestors
        GROUP BY folder_id
    )
    UPDATE teldrive.files f
    SET size = COALESCE(f.size, 0) + d.delta
    FROM deltas d
    WHERE f.id = d.folder_id;

    RETURN NULL;
END;
$$;

CREATE OR REPLACE FUNCTION teldrive.files_folder_size_delete_stmt_trigger()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    IF pg_trigger_depth() > 1 THEN
        RETURN NULL;
    END IF;

    WITH RECURSIVE changed AS (
        SELECT o.parent_id AS start_folder_id, -SUM(COALESCE(o.size, 0)) AS delta
        FROM old_rows o
        WHERE o.type = 'file'
          AND o.status = 'active'
          AND o.parent_id IS NOT NULL
        GROUP BY o.parent_id
    ),
    recursive_ancestors AS (
        SELECT f.id AS folder_id, f.parent_id, c.delta
        FROM changed c
        JOIN teldrive.files f ON f.id = c.start_folder_id
        WHERE f.type = 'folder'

        UNION ALL

        SELECT p.id AS folder_id, p.parent_id, ra.delta
        FROM recursive_ancestors ra
        JOIN teldrive.files p ON p.id = ra.parent_id
        WHERE p.type = 'folder'
    ),
    deltas AS (
        SELECT folder_id, SUM(delta) AS delta
        FROM recursive_ancestors
        GROUP BY folder_id
    )
    UPDATE teldrive.files f
    SET size = COALESCE(f.size, 0) + d.delta
    FROM deltas d
    WHERE f.id = d.folder_id;

    RETURN NULL;
END;
$$;

CREATE OR REPLACE FUNCTION teldrive.files_folder_size_update_stmt_trigger()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    IF pg_trigger_depth() > 1 THEN
        RETURN NULL;
    END IF;

    WITH RECURSIVE pairs AS (
        SELECT
            o.id,
            o.parent_id AS old_parent_id,
            n.parent_id AS new_parent_id,
            o.user_id AS old_user_id,
            n.user_id AS new_user_id,
            o.type AS old_type,
            n.type AS new_type,
            o.status AS old_status,
            n.status AS new_status,
            COALESCE(o.size, 0) AS old_size,
            COALESCE(n.size, 0) AS new_size
        FROM old_rows o
        JOIN new_rows n USING (id)
    ),
    file_delta_raw AS (
        SELECT
            CASE
                WHEN p.old_parent_id IS NOT DISTINCT FROM p.new_parent_id
                 AND p.old_user_id IS NOT DISTINCT FROM p.new_user_id
                THEN p.new_parent_id
                ELSE p.old_parent_id
            END AS start_folder_id,
            CASE
                WHEN p.old_parent_id IS NOT DISTINCT FROM p.new_parent_id
                 AND p.old_user_id IS NOT DISTINCT FROM p.new_user_id
                THEN
                    (CASE WHEN p.new_type = 'file' AND p.new_status = 'active' THEN p.new_size ELSE 0 END)
                    -
                    (CASE WHEN p.old_type = 'file' AND p.old_status = 'active' THEN p.old_size ELSE 0 END)
                ELSE
                    -(CASE WHEN p.old_type = 'file' AND p.old_status = 'active' THEN p.old_size ELSE 0 END)
            END AS delta
        FROM pairs p
        WHERE p.old_type = 'file' OR p.new_type = 'file'

        UNION ALL

        SELECT
            p.new_parent_id AS start_folder_id,
            (CASE WHEN p.new_type = 'file' AND p.new_status = 'active' THEN p.new_size ELSE 0 END) AS delta
        FROM pairs p
        WHERE (p.old_type = 'file' OR p.new_type = 'file')
          AND (p.old_parent_id IS DISTINCT FROM p.new_parent_id OR p.old_user_id IS DISTINCT FROM p.new_user_id)
    ),
    folder_move_delta_raw AS (
        SELECT p.old_parent_id AS start_folder_id, -p.new_size AS delta
        FROM pairs p
        WHERE p.old_type = 'folder'
          AND p.new_type = 'folder'
          AND (p.old_parent_id IS DISTINCT FROM p.new_parent_id OR p.old_user_id IS DISTINCT FROM p.new_user_id)

        UNION ALL

        SELECT p.new_parent_id AS start_folder_id, p.new_size AS delta
        FROM pairs p
        WHERE p.old_type = 'folder'
          AND p.new_type = 'folder'
          AND (p.old_parent_id IS DISTINCT FROM p.new_parent_id OR p.old_user_id IS DISTINCT FROM p.new_user_id)
    ),
    all_deltas AS (
        SELECT start_folder_id, delta FROM file_delta_raw
        UNION ALL
        SELECT start_folder_id, delta FROM folder_move_delta_raw
    ),
    changed AS (
        SELECT start_folder_id, SUM(delta) AS delta
        FROM all_deltas
        WHERE start_folder_id IS NOT NULL
        GROUP BY start_folder_id
        HAVING SUM(delta) <> 0
    ),
    recursive_ancestors AS (
        SELECT f.id AS folder_id, f.parent_id, c.delta
        FROM changed c
        JOIN teldrive.files f ON f.id = c.start_folder_id
        WHERE f.type = 'folder'

        UNION ALL

        SELECT p.id AS folder_id, p.parent_id, ra.delta
        FROM recursive_ancestors ra
        JOIN teldrive.files p ON p.id = ra.parent_id
        WHERE p.type = 'folder'
    ),
    deltas AS (
        SELECT folder_id, SUM(delta) AS delta
        FROM recursive_ancestors
        GROUP BY folder_id
    )
    UPDATE teldrive.files f
    SET size = COALESCE(f.size, 0) + d.delta
    FROM deltas d
    WHERE f.id = d.folder_id;

    RETURN NULL;
END;
$$;

CREATE TRIGGER trg_files_folder_size_insert_stmt
AFTER INSERT
ON teldrive.files
REFERENCING NEW TABLE AS new_rows
FOR EACH STATEMENT
EXECUTE FUNCTION teldrive.files_folder_size_insert_stmt_trigger();

CREATE TRIGGER trg_files_folder_size_delete_stmt
AFTER DELETE
ON teldrive.files
REFERENCING OLD TABLE AS old_rows
FOR EACH STATEMENT
EXECUTE FUNCTION teldrive.files_folder_size_delete_stmt_trigger();

CREATE TRIGGER trg_files_folder_size_update_stmt
AFTER UPDATE
ON teldrive.files
REFERENCING OLD TABLE AS old_rows NEW TABLE AS new_rows
FOR EACH STATEMENT
EXECUTE FUNCTION teldrive.files_folder_size_update_stmt_trigger();
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TRIGGER IF EXISTS trg_files_folder_size_update_stmt ON teldrive.files;
DROP TRIGGER IF EXISTS trg_files_folder_size_delete_stmt ON teldrive.files;
DROP TRIGGER IF EXISTS trg_files_folder_size_insert_stmt ON teldrive.files;

DROP FUNCTION IF EXISTS teldrive.files_folder_size_update_stmt_trigger();
DROP FUNCTION IF EXISTS teldrive.files_folder_size_delete_stmt_trigger();
DROP FUNCTION IF EXISTS teldrive.files_folder_size_insert_stmt_trigger();

-- Restore previous row-level trigger behavior.
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

CREATE TRIGGER trg_files_folder_size
AFTER INSERT OR DELETE OR UPDATE OF parent_id, size, status, type, user_id
ON teldrive.files
FOR EACH ROW
EXECUTE FUNCTION teldrive.files_folder_size_trigger();
-- +goose StatementEnd
