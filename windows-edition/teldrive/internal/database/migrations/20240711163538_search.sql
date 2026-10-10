-- +goose Up
-- +goose StatementBegin
DO $search$ BEGIN
IF EXISTS (SELECT 1 FROM pg_available_extensions WHERE name = 'pgroonga') THEN
CREATE EXTENSION IF NOT EXISTS pgroonga;
END IF;
END $search$;
DROP INDEX IF EXISTS teldrive.name_search_idx;
DROP FUNCTION IF EXISTS  teldrive.get_tsquery;
DROP FUNCTION IF EXISTS teldrive.get_tsvector;
DO $search$ BEGIN
IF EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'pgroonga') THEN
CREATE INDEX name_search_idx ON teldrive.files USING pgroonga (REGEXP_REPLACE(name, '[.,-_]', ' ', 'g')) WITH (tokenizer = 'TokenNgram');
END IF;
END $search$;
-- +goose StatementEnd
