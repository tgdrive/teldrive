-- +goose Up

DROP INDEX /* TEMPLATE: schema */files_unique_active_root_name_idx;
DROP INDEX /* TEMPLATE: schema */files_unique_active_child_name_idx;
DROP INDEX /* TEMPLATE: schema */files_list_idx;
DROP INDEX /* TEMPLATE: schema */files_name_search_idx;

ALTER TABLE /* TEMPLATE: schema */files
    DROP COLUMN normalized_name;

ALTER TABLE /* TEMPLATE: schema */upload_sessions
    DROP COLUMN normalized_name;

CREATE UNIQUE INDEX files_unique_active_root_name_idx
    ON /* TEMPLATE: schema */files (user_id, name)
    WHERE parent_id IS NULL AND status = 'active';

CREATE UNIQUE INDEX files_unique_active_child_name_idx
    ON /* TEMPLATE: schema */files (user_id, parent_id, name)
    WHERE parent_id IS NOT NULL AND status = 'active';

CREATE INDEX files_list_idx
    ON /* TEMPLATE: schema */files (user_id, parent_id, status, kind, name, id);

CREATE INDEX files_name_search_idx
    ON /* TEMPLATE: schema */files USING gin (name gin_trgm_ops);

-- +goose Down

DROP INDEX /* TEMPLATE: schema */files_unique_active_root_name_idx;
DROP INDEX /* TEMPLATE: schema */files_unique_active_child_name_idx;
DROP INDEX /* TEMPLATE: schema */files_list_idx;
DROP INDEX /* TEMPLATE: schema */files_name_search_idx;

ALTER TABLE /* TEMPLATE: schema */files
    ADD COLUMN normalized_name TEXT;

UPDATE /* TEMPLATE: schema */files
SET normalized_name = name;

ALTER TABLE /* TEMPLATE: schema */files
    ALTER COLUMN normalized_name SET NOT NULL,
    ADD CONSTRAINT files_normalized_name_not_blank CHECK (length(normalized_name) > 0);

ALTER TABLE /* TEMPLATE: schema */upload_sessions
    ADD COLUMN normalized_name TEXT;

UPDATE /* TEMPLATE: schema */upload_sessions
SET normalized_name = name;

ALTER TABLE /* TEMPLATE: schema */upload_sessions
    ALTER COLUMN normalized_name SET NOT NULL,
    ADD CONSTRAINT upload_sessions_normalized_name_not_blank CHECK (length(normalized_name) > 0);

CREATE UNIQUE INDEX files_unique_active_root_name_idx
    ON /* TEMPLATE: schema */files (user_id, normalized_name)
    WHERE parent_id IS NULL AND status = 'active';

CREATE UNIQUE INDEX files_unique_active_child_name_idx
    ON /* TEMPLATE: schema */files (user_id, parent_id, normalized_name)
    WHERE parent_id IS NOT NULL AND status = 'active';

CREATE INDEX files_list_idx
    ON /* TEMPLATE: schema */files (user_id, parent_id, status, kind, normalized_name, id);

CREATE INDEX files_name_search_idx
    ON /* TEMPLATE: schema */files USING gin (normalized_name gin_trgm_ops);
