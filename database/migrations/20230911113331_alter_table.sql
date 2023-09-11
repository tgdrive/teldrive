-- +goose Up
-- +goose StatementBegin

ALTER TABLE teldrive.files DROP CONSTRAINT unique_file;

ALTER TABLE teldrive.files ADD CONSTRAINT unique_file UNIQUE (name, parent_id, user_id,status)

-- +goose StatementEnd