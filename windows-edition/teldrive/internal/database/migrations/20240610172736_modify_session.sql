-- +goose Up
-- +goose StatementBegin
truncate teldrive.sessions;
ALTER TABLE "teldrive"."sessions" ADD COLUMN "auth_hash" text;
-- +goose StatementEnd