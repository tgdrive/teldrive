-- +goose Up
-- +goose StatementBegin
DROP INDEX IF EXISTS teldrive.idx_files_unique_file;

-- +goose StatementEnd
