-- +goose Up

-- Numeric collation compares digit runs numerically (file2 before file10).
-- Keep deterministic bytewise tie-breaking for distinct names such as file02
-- and file2; UUIDs then break ties between identical names across folders.
CREATE COLLATION IF NOT EXISTS /* TEMPLATE: schema */natural_name (
    provider = icu,
    locale = 'und-u-kn-true',
    deterministic = true
);

DROP INDEX /* TEMPLATE: schema */files_list_idx;
CREATE INDEX files_list_idx
    ON /* TEMPLATE: schema */files (user_id, parent_id, status, kind, name COLLATE /* TEMPLATE: schema */natural_name, id);

-- +goose Down

DROP INDEX /* TEMPLATE: schema */files_list_idx;
CREATE INDEX files_list_idx
    ON /* TEMPLATE: schema */files (user_id, parent_id, status, kind, name, id);

DROP COLLATION /* TEMPLATE: schema */natural_name;
