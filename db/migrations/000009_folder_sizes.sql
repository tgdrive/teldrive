-- +goose Up

-- Folders now carry their cumulative active-file size in size, maintained by
-- the teldrive-folder-size-rollup periodic River job. New folders start at 0.
ALTER TABLE /* TEMPLATE: schema */files
    DROP CONSTRAINT files_kind_size_check;
ALTER TABLE /* TEMPLATE: schema */files
    ADD CONSTRAINT files_kind_size_check CHECK (
        (kind = 'folder' AND (size IS NULL OR size >= 0))
        OR (kind = 'file' AND size IS NOT NULL AND size >= 0)
    );
UPDATE /* TEMPLATE: schema */files
SET size = 0
WHERE kind = 'folder'
  AND size IS NULL;

-- +goose Down

UPDATE /* TEMPLATE: schema */files
SET size = NULL
WHERE kind = 'folder';
ALTER TABLE /* TEMPLATE: schema */files
    DROP CONSTRAINT files_kind_size_check;
ALTER TABLE /* TEMPLATE: schema */files
    ADD CONSTRAINT files_kind_size_check CHECK (
        (kind = 'folder' AND size IS NULL)
        OR (kind = 'file' AND size IS NOT NULL AND size >= 0)
    );
