-- +goose Up
-- +goose StatementBegin
ALTER TABLE teldrive.uploads ADD COLUMN IF NOT EXISTS block_hashes BYTEA;
ALTER TABLE teldrive.files ADD COLUMN IF NOT EXISTS hash TEXT;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE teldrive.uploads DROP COLUMN IF EXISTS block_hashes;
ALTER TABLE teldrive.files DROP COLUMN IF EXISTS hash;
-- +goose StatementEnd
