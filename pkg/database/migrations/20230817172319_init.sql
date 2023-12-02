-- +goose Up

CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE EXTENSION IF NOT EXISTS btree_gin;

CREATE SCHEMA IF NOT EXISTS teldrive;

CREATE COLLATION IF NOT EXISTS numeric (PROVIDER = ICU, LOCALE = 'en@colnumeric=yes');

-- +goose StatementBegin

CREATE OR REPLACE
FUNCTION teldrive.generate_uid(size INT) RETURNS TEXT LANGUAGE PLPGSQL AS $$
DECLARE
    characters TEXT := 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789';
    bytes BYTEA := gen_random_bytes(size);
    l INT := LENGTH(characters);
    i INT := 0;
    output TEXT := '';
BEGIN
    WHILE i < size LOOP
        output := output || SUBSTR(characters, GET_BYTE(bytes, i) % l + 1, 1);
        i := i + 1;
    END LOOP;
    RETURN output;
END;
$$;

CREATE OR REPLACE
FUNCTION teldrive.get_tsvector(t TEXT) RETURNS TSVECTOR LANGUAGE PLPGSQL IMMUTABLE AS $$
DECLARE
    res TSVECTOR := to_tsvector(regexp_replace(t, '[^A-Za-z0-9 ]', ' ', 'g'));
BEGIN
    RETURN res;
END;
$$;

CREATE OR REPLACE
FUNCTION teldrive.get_tsquery(t TEXT) RETURNS TSQUERY LANGUAGE PLPGSQL IMMUTABLE AS $$
DECLARE
    res TSQUERY := CONCAT(plainto_tsquery(regexp_replace(t, '[^A-Za-z0-9 ]', ' ', 'g')), ':*')::TSQUERY;
BEGIN
    RETURN res;
END;
$$;
-- +goose StatementEnd


-- +goose Down
-- +goose StatementBegin
DROP FUNCTION IF EXISTS teldrive.generate_uid;
DROP FUNCTION IF EXISTS teldrive.get_tsvector;
DROP FUNCTION IF EXISTS  teldrive.get_tsquery;
-- +goose StatementEnd
