-- +goose Up
create table teldrive.files (
id text primary key not null default teldrive.generate_uid(16),
name text not null,
type text not null,
mime_type text not null,
path text null,
size bigint null,
starred bool not null,
depth integer null,
user_id bigint not null,
parent_id text null,
status text default 'active'::text,
channel_id bigint null,
parts jsonb null,
created_at timestamp not null default timezone('utc'::text,
now()),
updated_at timestamp not null default timezone('utc'::text,
now()),
constraint unique_file unique (name,
parent_id,user_id)
);

create table teldrive.uploads (
	id text not null primary key default teldrive.generate_uid(16),
	upload_id text not null,
	name text not null,
	part_no int4 not null,
	part_id int4 not null,
	total_parts int4 not null,
	channel_id int8 not null,
	size int8 not null,
	created_at timestamp null default timezone('utc'::text,
now())
);

create table teldrive.users (
	user_id int4 not null primary key,
	name text null,
	user_name text null,
	is_premium bool not null,
	tg_session text not null,
	settings jsonb null,
	created_at timestamptz not null default timezone('utc'::text,
now()),
	updated_at timestamptz not null default timezone('utc'::text,
now())
);

create collation if not exists numeric (provider = icu, locale = 'en@colnumeric=yes');
create index  name_search_idx on
teldrive.files
	using gin (teldrive.get_tsvector(name),
updated_at);

create index  name_numeric_idx on
teldrive.files(name collate numeric nulls first);

create index  parent_name_numeric_idx on
teldrive.files (parent_id,
name collate numeric desc);

create index  path_idx on
teldrive.files (path);

create index  parent_idx on
teldrive.files (parent_id);

create index  starred_updated_at_idx on
teldrive.files (starred,
updated_at desc);

create index  status_idx on teldrive.files (status);

create index  user_id_idx on teldrive.files (user_id);

-- +goose Down
drop table if exists teldrive.files;
drop table if exists teldrive.uploads;
drop table if exists teldrive.users;
drop index if exists teldrive.name_search_idx;
drop index if exists teldrive.name_numeric_idx;
drop index if exists teldrive.parent_name_numeric_idx;
drop index if exists teldrive.path_idx;
drop index if exists teldrive.parent_idx;
drop index if exists teldrive.starred_updated_at_idx;
drop index if exists teldrive.status_idx;
drop index if exists teldrive.user_id_idx;