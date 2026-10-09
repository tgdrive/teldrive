-- +goose Up
-- +goose StatementBegin
ALTER TABLE teldrive.files ADD COLUMN IF NOT EXISTS lifecycle_root uuid;
ALTER TABLE teldrive.files ADD COLUMN IF NOT EXISTS lifecycle_at timestamptz;
CREATE INDEX IF NOT EXISTS idx_files_lifecycle ON teldrive.files(user_id, status, lifecycle_at) WHERE lifecycle_at IS NOT NULL;
DROP INDEX IF EXISTS teldrive.idx_files_unique_folder;
CREATE UNIQUE INDEX idx_files_unique_folder ON teldrive.files(name, parent_id, user_id) WHERE type = 'folder' AND status = 'active';
-- +goose StatementEnd
