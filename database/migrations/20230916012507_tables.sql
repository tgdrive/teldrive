-- +goose Up
-- +goose StatementBegin

CREATE TABLE teldrive.bots (
    user_id bigint NOT NULL,
    token text NOT NULL PRIMARY KEY,
    bot_user_name text NOT NULL,
    bot_id bigint NOT NULL,
    FOREIGN KEY (user_id) REFERENCES teldrive.users(user_id)
);

CREATE TABLE teldrive.channels (
    channel_id bigint NOT NULL PRIMARY KEY,
    channel_name text NOT NULL,
    user_id bigint NOT NULL,
    selected boolean DEFAULT false,
    FOREIGN KEY (user_id) REFERENCES teldrive.users(user_id)
);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS teldrive.bots;
DROP TABLE IF EXISTS teldrive.channels;
-- +goose StatementEnd
