-- +goose Up
-- +goose StatementBegin
ALTER TABLE teldrive.files ADD COLUMN "encrypted" BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE teldrive.uploads ADD COLUMN "encrypted" BOOLEAN NOT NULL DEFAULT FALSE;
-- +goose StatementEnd