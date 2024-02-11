-- +goose Up
-- +goose StatementBegin
ALTER TABLE teldrive.files ADD COLUMN IF NOT EXISTS "encrypted" BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE teldrive.uploads ADD COLUMN IF NOT EXISTS "encrypted" BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE teldrive.uploads ADD COLUMN IF NOT EXISTS "salt" TEXT;
-- +goose StatementEnd