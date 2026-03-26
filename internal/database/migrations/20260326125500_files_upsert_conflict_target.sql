-- +goose Up
-- +goose StatementBegin
DROP INDEX IF EXISTS teldrive.unique_file;
DROP INDEX IF EXISTS teldrive.unique_folder;
DROP INDEX IF EXISTS teldrive.idx_files_unique_file;
DROP INDEX IF EXISTS teldrive.idx_files_unique_folder;
DROP INDEX IF EXISTS teldrive.unique_file_idx;

ALTER TABLE teldrive.files
  ADD CONSTRAINT files_unique_active_entry
  UNIQUE NULLS NOT DISTINCT (name, parent_id, user_id, status, type);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE teldrive.files
  DROP CONSTRAINT IF EXISTS files_unique_active_entry;

CREATE UNIQUE INDEX IF NOT EXISTS unique_file_idx ON teldrive.files
  (name, COALESCE(parent_id, '00000000-0000-0000-0000-000000000000'::uuid), user_id)
  WHERE status = 'active';
-- +goose StatementEnd
