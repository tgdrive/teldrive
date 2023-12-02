-- +goose Up

ALTER TABLE teldrive.files DROP CONSTRAINT IF EXISTS unique_file;

ALTER TABLE teldrive.users DROP COLUMN settings;

CREATE UNIQUE INDEX unique_file ON teldrive.files (name, parent_id, user_id) WHERE (status= 'active');
