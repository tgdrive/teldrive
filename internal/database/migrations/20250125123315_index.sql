-- +goose Up
-- +goose StatementBegin
DROP INDEX IF EXISTS teldrive.idx_files_unique_folder;
CREATE UNIQUE INDEX unique_file_idx ON teldrive.files 
(name, COALESCE(parent_id, '00000000-0000-0000-0000-000000000000'::uuid), user_id) WHERE status = 'active';
-- +goose StatementEnd
