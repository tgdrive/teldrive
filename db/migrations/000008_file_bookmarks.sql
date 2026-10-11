-- +goose Up

CREATE TABLE /* TEMPLATE: schema */file_bookmarks (
    user_id BIGINT NOT NULL,
    file_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, file_id),
    FOREIGN KEY (file_id, user_id)
        REFERENCES /* TEMPLATE: schema */files(id, user_id)
        ON DELETE CASCADE
);

CREATE INDEX file_bookmarks_user_created_idx
    ON /* TEMPLATE: schema */file_bookmarks (user_id, created_at DESC, file_id DESC);

CREATE INDEX file_bookmarks_file_user_idx
    ON /* TEMPLATE: schema */file_bookmarks (file_id, user_id);

-- +goose Down

DROP TABLE IF EXISTS /* TEMPLATE: schema */file_bookmarks;
