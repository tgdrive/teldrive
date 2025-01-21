-- +goose Up
-- +goose StatementBegin
CREATE INDEX IF NOT EXISTS idx_files_user_channel_type_id ON teldrive.files (user_id, channel_id, type, id);
-- +goose StatementEnd