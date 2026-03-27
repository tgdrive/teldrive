-- +goose Up
-- +goose StatementBegin
DROP INDEX IF EXISTS teldrive.unique_file;
DROP INDEX IF EXISTS teldrive.unique_folder;
DROP INDEX IF EXISTS teldrive.idx_files_unique_file;
DROP INDEX IF EXISTS teldrive.idx_files_unique_folder;
DROP INDEX IF EXISTS teldrive.unique_file_idx;

CREATE UNIQUE INDEX files_unique_active_entry
  ON teldrive.files (name, parent_id, user_id) NULLS NOT DISTINCT
  WHERE status = 'active';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS teldrive.files_unique_active_entry;

CREATE UNIQUE INDEX IF NOT EXISTS unique_file_idx ON teldrive.files
  (name, COALESCE(parent_id, '00000000-0000-0000-0000-000000000000'::uuid), user_id)
  WHERE status = 'active';
-- +goose StatementEnd
