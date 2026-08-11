-- +goose Up

CREATE TABLE /* TEMPLATE: schema */file_view_states (
    user_id BIGINT NOT NULL REFERENCES /* TEMPLATE: schema */users(user_id) ON DELETE CASCADE,
    file_id UUID NOT NULL,
    viewer_kind TEXT NOT NULL,
    position JSONB NOT NULL DEFAULT '{}'::jsonb,
    preferences JSONB NOT NULL DEFAULT '{}'::jsonb,
    bookmarks JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, file_id),
    FOREIGN KEY (file_id, user_id)
        REFERENCES /* TEMPLATE: schema */files(id, user_id)
        ON DELETE CASCADE,
    CONSTRAINT file_view_states_kind_valid CHECK (
        viewer_kind IN ('image', 'video', 'audio', 'pdf', 'ebook', 'text')
    ),
    CONSTRAINT file_view_states_position_object CHECK (jsonb_typeof(position) = 'object'),
    CONSTRAINT file_view_states_preferences_object CHECK (jsonb_typeof(preferences) = 'object'),
    CONSTRAINT file_view_states_bookmarks_array CHECK (jsonb_typeof(bookmarks) = 'array'),
    CONSTRAINT file_view_states_bookmarks_limit CHECK (jsonb_array_length(bookmarks) <= 500)
);

-- +goose Down

DROP TABLE IF EXISTS /* TEMPLATE: schema */file_view_states;
