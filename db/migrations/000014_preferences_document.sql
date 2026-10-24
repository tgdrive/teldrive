-- +goose Up
-- Intentional clean start: only UI preferences are discarded. New settings and
-- their defaults belong to the application, not additional database columns.
DROP TABLE /* TEMPLATE: schema */user_preferences;
CREATE TABLE /* TEMPLATE: schema */user_preferences (
    user_id BIGINT PRIMARY KEY REFERENCES /* TEMPLATE: schema */users(user_id) ON DELETE CASCADE,
    preferences JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ,
    CONSTRAINT user_preferences_document_object CHECK (jsonb_typeof(preferences) = 'object')
);

-- +goose Down
-- Rollback is also a clean start; discarded preference data cannot be restored.
DROP TABLE /* TEMPLATE: schema */user_preferences;
CREATE TABLE /* TEMPLATE: schema */user_preferences (
    user_id BIGINT PRIMARY KEY REFERENCES /* TEMPLATE: schema */users(user_id) ON DELETE CASCADE,
    upload_encryption BOOLEAN NOT NULL DEFAULT false,
    upload_conflict_policy /* TEMPLATE: schema */name_conflict_policy NOT NULL DEFAULT 'rename',
    upload_concurrency INTEGER NOT NULL DEFAULT 3 CHECK (upload_concurrency BETWEEN 1 AND 12),
    upload_part_size BIGINT NOT NULL DEFAULT 536870912 CHECK (upload_part_size BETWEEN 16777216 AND 2147483648 AND upload_part_size % 16777216 = 0),
    file_sort TEXT NOT NULL DEFAULT 'name' CHECK (file_sort IN ('name', 'updatedAt', 'size')),
    file_order TEXT NOT NULL DEFAULT 'asc' CHECK (file_order IN ('asc', 'desc')),
    theme TEXT NOT NULL DEFAULT 'system' CHECK (theme IN ('system', 'light', 'dark')),
    locale TEXT CHECK (locale IS NULL OR char_length(locale) BETWEEN 2 AND 16),
    embedded_tracks BOOLEAN NOT NULL DEFAULT false,
    embedded_tracks_on_shares BOOLEAN NOT NULL DEFAULT false CHECK (NOT embedded_tracks_on_shares OR embedded_tracks),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ
);
