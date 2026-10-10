-- +goose Up
-- +goose StatementBegin
ALTER TABLE teldrive.bots DROP CONSTRAINT IF EXISTS btoken_user_channel_un;
ALTER TABLE teldrive.bots DROP CONSTRAINT IF EXISTS bots_pkey;
ALTER TABLE teldrive.bots ADD CONSTRAINT bots_pkey PRIMARY KEY (user_id, token, channel_id);
-- +goose StatementEnd