-- +goose Up

-- Allow open-ended upload sessions to use -1 until completion derives the size.
ALTER TABLE /* TEMPLATE: schema */upload_sessions
    DROP CONSTRAINT upload_sessions_expected_size_nonnegative,
    ADD CONSTRAINT upload_sessions_expected_size_valid CHECK (expected_size >= -1);

-- +goose Down

ALTER TABLE /* TEMPLATE: schema */upload_sessions
    DROP CONSTRAINT upload_sessions_expected_size_valid,
    ADD CONSTRAINT upload_sessions_expected_size_nonnegative CHECK (expected_size >= 0);
