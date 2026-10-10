-- +goose Up
-- +goose StatementBegin
truncate teldrive.sessions;
ALTER TABLE "teldrive"."sessions" DROP COLUMN IF EXISTS "auth_hash";
ALTER TABLE "teldrive"."sessions" ADD COLUMN "session_date" integer;
-- +goose StatementEnd