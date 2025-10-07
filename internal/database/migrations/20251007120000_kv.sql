-- +goose Up
-- +goose StatementBegin


CREATE TEMP TABLE bots_temp AS
SELECT DISTINCT ON (user_id, token)
    user_id,
    token,
    bot_id
FROM
    teldrive.bots
ORDER BY
    user_id, token, bot_id;

DROP TABLE teldrive.bots;

CREATE TABLE teldrive.bots (
	user_id int8 NOT NULL,
	token text NOT NULL,
	bot_id int8 NOT NULL,
	CONSTRAINT bots_pkey PRIMARY KEY (user_id, token),
	CONSTRAINT bots_user_id_fkey FOREIGN KEY (user_id) REFERENCES teldrive.users(user_id)
);

INSERT INTO teldrive.bots (user_id, token, bot_id)
SELECT user_id, token, bot_id FROM bots_temp;

CREATE  TABLE IF NOT EXISTS teldrive.kv (
    key text PRIMARY KEY,
    value BYTEA NOT NULL,
    created_at TIMESTAMP DEFAULT timezone('utc'::text, now()) NOT NULL
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS teldrive.kv;
-- +goose StatementEnd
