-- +goose Up
-- +goose StatementBegin
ALTER TABLE teldrive.bots DROP CONSTRAINT IF EXISTS btoken_user_un;
ALTER TABLE teldrive.bots DROP CONSTRAINT IF EXISTS btoken_user_channel_un;
ALTER TABLE teldrive.bots ADD CONSTRAINT btoken_user_channel_un UNIQUE (user_id,token,channel_id);
-- +goose StatementEnd