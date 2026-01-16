-- +goose Up
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION teldrive.clean_name(val text) RETURNS text AS $$
BEGIN
  RETURN lower(regexp_replace(val, '[^[:alnum:]\s]', ' ', 'g'));
END;
$$ LANGUAGE plpgsql IMMUTABLE;
-- +goose StatementEnd

-- Performance Indexes
CREATE INDEX IF NOT EXISTS idx_files_channel_id ON teldrive.files (channel_id);
CREATE INDEX IF NOT EXISTS idx_files_parent_name_lookup ON teldrive.files (parent_id, name, status);

-- Browsing (Covering for Deferred Join)
CREATE INDEX IF NOT EXISTS idx_files_browsing ON teldrive.files (user_id, parent_id, status, updated_at DESC) INCLUDE (id);

-- Count Optimization
CREATE INDEX IF NOT EXISTS idx_files_user_active ON teldrive.files (user_id) WHERE status = 'active';

-- Size & Category Sorting/Filtering
CREATE INDEX IF NOT EXISTS idx_files_size ON teldrive.files (user_id, status, size DESC);
CREATE INDEX IF NOT EXISTS idx_files_category ON teldrive.files (user_id, status, category);

-- Created At & Type
CREATE INDEX IF NOT EXISTS idx_files_created_at ON teldrive.files (user_id, status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_files_type ON teldrive.files (user_id, status, type);

-- Search (Using Function)
DROP INDEX IF EXISTS teldrive.idx_files_name_search;
CREATE INDEX idx_files_name_search ON teldrive.files USING pgroonga (teldrive.clean_name(name)) WITH (tokenizer='TokenNgram');

-- Rclone / UUIDv7 Optimization (Covering)
CREATE INDEX IF NOT EXISTS idx_files_id_browsing ON teldrive.files (user_id, parent_id, status, id DESC) INCLUDE (name, size, mime_type, type);

-- +goose Down
DROP INDEX IF EXISTS teldrive.idx_files_channel_id;
DROP INDEX IF EXISTS teldrive.idx_files_parent_name_lookup;
DROP INDEX IF EXISTS teldrive.idx_files_browsing;
DROP INDEX IF EXISTS teldrive.idx_files_user_active;
DROP INDEX IF EXISTS teldrive.idx_files_size;
DROP INDEX IF EXISTS teldrive.idx_files_category;
DROP INDEX IF EXISTS teldrive.idx_files_created_at;
DROP INDEX IF EXISTS teldrive.idx_files_type;
DROP INDEX IF EXISTS teldrive.idx_files_name_search;
DROP INDEX IF EXISTS teldrive.idx_files_id_browsing;
DROP FUNCTION IF EXISTS teldrive.clean_name(text);
