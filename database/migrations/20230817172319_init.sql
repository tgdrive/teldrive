-- +goose Up
create extension if not exists pgcrypto;

create extension if not exists btree_gin;

create schema if not exists teldrive;

create collation if not exists numeric (provider = icu, locale = 'en@colnumeric=yes');

-- +goose StatementBegin
create or replace
function teldrive.generate_uid(size int) returns text language plpgsql as $$
declare characters text := 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789';

bytes bytea := gen_random_bytes(size);

l int := length(characters);

i int := 0;

output text := '';

begin while i < size loop output := output || substr(characters,
get_byte(bytes,
i) % l + 1,
1);

i := i + 1;
end loop;

return output;
end;

$$;
-- +goose StatementEnd

-- +goose StatementBegin
create or replace
function teldrive.get_tsvector(t text) returns tsvector language plpgsql immutable as $$
declare res tsvector := to_tsvector(regexp_replace(t,
'[^A-Za-z0-9 ]',
' ',
'g'));

begin return res;
end;

$$;
-- +goose StatementEnd

-- +goose StatementBegin
create or replace
function teldrive.get_tsquery(t text) returns tsquery language plpgsql immutable as $$
declare res tsquery = concat(
plainto_tsquery(regexp_replace(t,
'[^A-Za-z0-9 ]',
' ',
'g')),
':*'
)::tsquery;

begin return res;
end;

$$;
-- +goose StatementEnd
