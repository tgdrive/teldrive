-- +goose Up

CREATE TABLE IF NOT EXISTS teldrive.files (
	id text NOT NULL DEFAULT teldrive.generate_uid(16) PRIMARY KEY,
	"name" text NOT NULL,
	"type" text NOT NULL,
	mime_type text NOT NULL,
	"path" text,
	"size" bigint,
	starred bool NOT NULL,
	"depth" int,
	user_id bigint NOT NULL,
	parent_id text,
	status text DEFAULT 'active'::text,
	channel_id bigint,
	parts jsonb,
	created_at timestamp NOT NULL DEFAULT timezone('utc'::text, now()),
	updated_at timestamp NOT NULL DEFAULT timezone('utc'::text, now())
);

CREATE TABLE IF NOT EXISTS teldrive.users (
	user_id bigint NOT NULL PRIMARY KEY,
	"name" text,
	user_name text NOT NULL,
	is_premium bool NOT NULL,
	created_at timestamptz NOT NULL DEFAULT timezone('utc'::text, now()),
	updated_at timestamptz NOT NULL DEFAULT timezone('utc'::text, now())
);


CREATE TABLE IF NOT EXISTS teldrive.uploads (
	upload_id text NOT NULL,
	"name" text NOT NULL,
    user_id bigint,
	part_no int NOT NULL,
	part_id int NOT NULL PRIMARY KEY,
	channel_id bigint NOT NULL,
	"size" bigint NOT NULL,
	created_at timestamp DEFAULT timezone('utc'::text, now()),
	CONSTRAINT part_id_greater_than_zero CHECK (part_id > 0)
);


CREATE TABLE IF NOT EXISTS teldrive.channels (
	channel_id bigint NOT NULL PRIMARY KEY,
	channel_name text NOT NULL,
	user_id bigint NOT NULL,
	selected boolean DEFAULT false,
    FOREIGN KEY (user_id) REFERENCES teldrive.users(user_id)
);

CREATE TABLE IF NOT EXISTS teldrive.bots (
	user_id bigint NOT NULL,
	"token" text NOT NULL,
	bot_user_name text NOT NULL,
	bot_id bigint NOT NULL,
	channel_id bigint NULL,
	FOREIGN KEY (user_id) REFERENCES teldrive.users(user_id),
    CONSTRAINT btoken_user_un  UNIQUE (user_id,token)
);

CREATE TABLE IF NOT EXISTS teldrive.sessions (
    session text NOT NULL,
    user_id bigint NOT NULL,
    hash text NOT NULL,
    created_at timestamp default timezone('utc'::text,now()),
    PRIMARY KEY(session, hash),
    FOREIGN KEY (user_id) REFERENCES teldrive.users(user_id)
);

CREATE INDEX IF NOT EXISTS name_numeric_idx ON teldrive.files USING btree (name COLLATE "numeric" NULLS FIRST);
CREATE INDEX IF NOT EXISTS name_search_idx ON teldrive.files USING gin (teldrive.get_tsvector(name), updated_at);
CREATE INDEX IF NOT EXISTS parent_idx ON teldrive.files USING btree (parent_id);
CREATE INDEX IF NOT EXISTS parent_name_numeric_idx ON teldrive.files USING btree (parent_id, name COLLATE "numeric" DESC);
CREATE INDEX IF NOT EXISTS path_idx ON teldrive.files USING btree (path);
CREATE INDEX IF NOT EXISTS starred_updated_at_idx ON teldrive.files USING btree (starred, updated_at DESC);
CREATE INDEX IF NOT EXISTS status_idx ON teldrive.files USING btree (status);
CREATE UNIQUE INDEX IF NOT EXISTS unique_file ON teldrive.files USING btree (name, parent_id, user_id) WHERE (status = 'active'::text);
CREATE INDEX IF NOT EXISTS user_id_idx ON teldrive.files USING btree (user_id);

-- +goose Down

DROP INDEX IF EXISTS name_numeric_idx ;
DROP INDEX IF EXISTS name_search_idx ;
DROP INDEX IF EXISTS parent_idx ;
DROP INDEX IF EXISTS parent_name_numeric_idx ;
DROP INDEX IF EXISTS path_idx ;
DROP INDEX IF EXISTS starred_updated_at_idx ;
DROP INDEX IF EXISTS status_idx ;
DROP INDEX IF EXISTS unique_file;
DROP INDEX IF EXISTS user_id_idx;

DROP TABLE IF EXISTS teldrive.files;
DROP TABLE IF EXISTS teldrive.uploads;
DROP TABLE IF EXISTS teldrive.users;
DROP TABLE IF EXISTS teldrive.channels;
DROP TABLE IF EXISTS teldrive.bots;
DROP TABLE IF EXISTS teldrive.sessions;