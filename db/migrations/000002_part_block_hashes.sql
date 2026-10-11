-- +goose Up

ALTER TABLE /* TEMPLATE: schema */upload_parts
    ADD COLUMN block_hashes BYTEA;

ALTER TABLE /* TEMPLATE: schema */file_parts
    ADD COLUMN block_hashes BYTEA;

COMMENT ON COLUMN /* TEMPLATE: schema */upload_parts.block_hashes IS
    'Concatenated 32-byte BLAKE3 hashes for each 16 MiB plaintext block in this upload part.';

COMMENT ON COLUMN /* TEMPLATE: schema */file_parts.block_hashes IS
    'Concatenated 32-byte BLAKE3 hashes for each 16 MiB plaintext block in this finalized part.';

-- +goose Down

ALTER TABLE /* TEMPLATE: schema */file_parts
    DROP COLUMN IF EXISTS block_hashes;

ALTER TABLE /* TEMPLATE: schema */upload_parts
    DROP COLUMN IF EXISTS block_hashes;
