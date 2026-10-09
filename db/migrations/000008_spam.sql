-- +goose Up
ALTER TYPE /* TEMPLATE: schema */file_status ADD VALUE IF NOT EXISTS 'spam';

-- +goose Down
-- PostgreSQL enum values cannot be removed without rebuilding dependent columns.
