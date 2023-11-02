-- +goose Up
-- +goose StatementBegin

ALTER TABLE teldrive.users DROP COLUMN IF EXISTS tg_session;

CREATE TABLE teldrive.sessions (
    session text NOT NULL,
    user_id bigint NOT NULL,
    hash text NOT NULL,
    created_at timestamp null default timezone('utc'::text,now()),
    PRIMARY KEY(session, hash),
    FOREIGN KEY (user_id) REFERENCES teldrive.users(user_id)
);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS teldrive.sessions;
-- +goose StatementEnd
