-- +goose Up
-- +goose StatementBegin
DROP INDEX IF EXISTS teldrive.unique_file;
CREATE UNIQUE INDEX IF NOT EXISTS unique_file ON teldrive.files USING btree (name, parent_id, user_id,size) WHERE (status = 'active'::text);
-- +goose StatementEnd
