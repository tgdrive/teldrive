-- +goose Up
-- +goose StatementBegin
CREATE EXTENSION IF NOT EXISTS pgroonga;
DROP INDEX IF EXISTS teldrive.name_search_idx;
DROP FUNCTION IF EXISTS  teldrive.get_tsquery;
DROP FUNCTION IF EXISTS teldrive.get_tsvector;
CREATE INDEX name_search_idx ON teldrive.files USING pgroonga (REGEXP_REPLACE(name, '[.,-_]', ' ', 'g')) WITH (tokenizer = 'TokenNgram');
-- +goose StatementEnd