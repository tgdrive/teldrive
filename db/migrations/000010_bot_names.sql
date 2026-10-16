-- +goose Up
ALTER TABLE /* TEMPLATE: schema */bots ADD COLUMN name TEXT;

-- +goose Down
ALTER TABLE /* TEMPLATE: schema */bots DROP COLUMN name;
