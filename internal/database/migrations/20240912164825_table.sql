-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS teldrive.file_shares (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    file_id uuid NOT NULL,
    password text NULL,
    expires_at timestamp NULL,
    created_at timestamp NOT NULL DEFAULT timezone('utc'::text, now()),
    updated_at timestamp NOT NULL DEFAULT timezone('utc'::text, now()),
    user_id bigint NOT NULL,
    CONSTRAINT file_shares_pkey PRIMARY KEY (id),
    CONSTRAINT fk_file FOREIGN KEY (file_id) REFERENCES teldrive.files (id) ON DELETE CASCADE
);

CREATE INDEX idx_file_shares_file_id ON teldrive.file_shares USING btree (file_id);
ALTER TABLE teldrive.files DROP COLUMN IF EXISTS starred;

-- +goose StatementEnd
