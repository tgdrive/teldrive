-- +goose Up
-- +goose StatementBegin
ALTER TABLE teldrive.files
ADD COLUMN shared_token_id text DEFAULT NULL;
CREATE TABLE teldrive.shared_tokens (
    id TEXT PRIMARY KEY NOT NULL DEFAULT teldrive.generate_uid(16),
    token TEXT NOT NULL,
    file_id TEXT NOT NULL,
    user_id BIGINT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT timezone('utc'::text, now()),
    updated_at TIMESTAMP NOT NULL DEFAULT timezone('utc'::text, now()),
    CONSTRAINT unique_token_file UNIQUE (token,file_id)
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE teldrive.files
DROP COLUMN shared_token_id;

DROP TABLE IF EXISTS teldrive.shared_tokens
-- +goose StatementEnd
