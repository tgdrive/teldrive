-- +goose Up
-- +goose StatementBegin
DROP INDEX IF EXISTS teldrive.idx_files_name_search;
DO $search$ BEGIN
IF EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'pgroonga') THEN
CREATE INDEX IF NOT EXISTS idx_files_name_search ON teldrive.files USING pgroonga (lower(regexp_replace(name, '[^[:alnum:]\\s]', ' ', 'g'))) WITH (tokenizer='TokenNgram');
CREATE INDEX IF NOT EXISTS idx_files_name_regex_search ON teldrive.files USING pgroonga (name pgroonga_text_regexp_ops_v2);
END IF;
END $search$;

-- +goose StatementEnd
