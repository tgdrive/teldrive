-- +goose Up
-- +goose StatementBegin
DROP INDEX IF EXISTS teldrive.idx_files_name_search;
CREATE INDEX IF NOT EXISTS idx_files_name_search ON teldrive.files USING pgroonga (lower(regexp_replace(name, '[^[:alnum:]\\s]', ' ', 'g'))) WITH (tokenizer='TokenNgram');
CREATE INDEX IF NOT EXISTS idx_files_name_regex_search ON teldrive.files USING pgroonga (name pgroonga_text_regexp_ops_v2);

-- +goose StatementEnd