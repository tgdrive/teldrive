-- +goose Up

-- Web UI preferences that follow the account across browsers and devices.
-- Rows are created lazily on first read; column defaults are the server
-- defaults. updated_at stays NULL until the user changes a value, which lets
-- clients import settings they previously kept in browser storage.
CREATE TABLE /* TEMPLATE: schema */user_preferences (
    user_id BIGINT PRIMARY KEY REFERENCES /* TEMPLATE: schema */users(user_id) ON DELETE CASCADE,
    upload_encryption BOOLEAN NOT NULL DEFAULT false,
    upload_conflict_policy /* TEMPLATE: schema */name_conflict_policy NOT NULL DEFAULT 'rename',
    upload_concurrency INTEGER NOT NULL DEFAULT 3,
    upload_part_size BIGINT NOT NULL DEFAULT 536870912,
    file_sort TEXT NOT NULL DEFAULT 'name',
    file_order TEXT NOT NULL DEFAULT 'asc',
    theme TEXT NOT NULL DEFAULT 'system',
    -- NULL means the client detects the language from the browser. The API
    -- contract owns the list of supported locales.
    locale TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ,
    CONSTRAINT user_preferences_upload_concurrency_range CHECK (upload_concurrency BETWEEN 1 AND 12),
    CONSTRAINT user_preferences_upload_part_size_valid CHECK (
        upload_part_size BETWEEN 16777216 AND 2147483648
        AND upload_part_size % 16777216 = 0
    ),
    CONSTRAINT user_preferences_file_sort_valid CHECK (file_sort IN ('name', 'updatedAt', 'size')),
    CONSTRAINT user_preferences_file_order_valid CHECK (file_order IN ('asc', 'desc')),
    CONSTRAINT user_preferences_theme_valid CHECK (theme IN ('system', 'light', 'dark')),
    CONSTRAINT user_preferences_locale_length CHECK (locale IS NULL OR char_length(locale) BETWEEN 2 AND 16)
);

-- +goose Down

DROP TABLE IF EXISTS /* TEMPLATE: schema */user_preferences;
