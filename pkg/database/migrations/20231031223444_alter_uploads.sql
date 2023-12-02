-- +goose Up

ALTER TABLE teldrive.uploads ADD COLUMN user_id BIGINT;