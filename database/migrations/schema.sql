create extension if not exists pgcrypto;

create extension if not exists btree_gin;

create schema if not exists teldrive;

set
search_path = 'teldrive';

create or replace
function generate_uid(size int) returns text as $$
declare characters text := 'abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyz0123456789';

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

$$ language plpgsql volatile;

create or replace
function get_tsvector(t text) returns tsvector language plpgsql immutable as $$
declare res tsvector := to_tsvector(regexp_replace(t,
'[^a-za-z0-9 ]',
' ',
'g'));

begin return res;
end;

$$;

create or replace
function get_tsquery(t text) returns tsquery language plpgsql immutable as $$
declare res tsquery = concat(
plainto_tsquery(regexp_replace(t,
'[^a-za-z0-9 ]',
' ',
'g')),
':*'
)::tsquery;

begin return res;
end;

$$;

create table if not exists files (
id text primary key not null default generate_uid(16),
name text not null,
type text not null,
mime_type text not null,
path text null,
size bigint null,
starred bool not null,
depth integer null,
user_id bigint not null,
parent_id text null,
status text DEFAULT 'active'::text,
channel_id bigint null,
parts jsonb NULL,
created_at timestamp not null default timezone('utc'::text,
now()),
updated_at timestamp not null default timezone('utc'::text,
now()),
constraint unique_file unique (name,
parent_id,user_id)
);

create table uploads (
	id text not null primary key default generate_uid(16),
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


create or replace
procedure update_size() language plpgsql as $$
declare rec record;

total_size bigint;

begin

for rec in
select
	id
from
	files
where
	type = 'folder'
order by
	depth desc loop total_size := (
	select
		sum(size) as total_size
	from
		teldrive.files
	where
		parent_id = rec.id
);

update
	teldrive.files
set
	size = total_size
where
	id = rec.id;
end loop;
end;

$$;

create or replace
function teldrive.update_folder(folder_id text,
new_name text default null,
new_path text default null)
 returns setof teldrive.files
 language plpgsql
as $$
declare folder record;

path_items text [];

begin 

if new_path is null then
select
	*
into
	folder
from
	teldrive.files
where
	id = folder_id;

path_items := string_to_array(folder.path,
'/');

path_items [array_length(path_items,
1)] := new_name;

new_path := array_to_string(path_items,
'/');
end if;

update
	teldrive.files
set
	path = new_path,
	name = new_name
where
	id = folder_id;

for folder in
select
	*
from
	teldrive.files
where
	type = 'folder'
	and parent_id = folder_id loop call teldrive.update_folder(
	folder.id,
	folder.name,
	concat(new_path,
	'/',
	folder.name)
);
end loop;

return query
select
	*
from
	teldrive.files
where
	id = folder_id;
end;

$$
;

create or replace
procedure teldrive.delete_files(in file_ids text[],
in op text default 'bulk')
 language plpgsql
as $$
    declare 
    rec record;

begin
    if op = 'bulk' then
    for rec in
select
	id,
	type
from
	teldrive.files
where
	id = any (file_ids)
    loop 
	    if rec.type = 'folder' then
	    call teldrive.delete_files(array [rec.id],
	'single');

delete
from
	teldrive.files
where
	id = rec.id;
else
	    update
	teldrive.files
set
	status = 'pending_deletion'
where
	id = rec.id;
end if;
end loop;
else
   
   for rec in
select
	id,
	type
from
	teldrive.files
where
	parent_id = file_ids[1]
    loop 
	    if rec.type = 'folder' then
	    call teldrive.delete_files(array [rec.id],
	'single');

delete
from
	teldrive.files
where
	id = rec.id;
else
	    update
	teldrive.files
set
	status = 'pending_deletion'
where
	id = rec.id;
end if;
end loop;
end if;
end;

$$
;

create collation if not exists numeric (provider = icu, locale = 'en@colnumeric=yes');

create index if not exists name_search_idx on
files
	using gin (get_tsvector(name),
updated_at);

create index if not exists name_numeric_idx on
files(name collate numeric nulls first);

create index if not exists parent_name_numeric_idx on
files (parent_id,
name collate numeric desc);

create index if not exists path_idx on
files (path);

create index if not exists parent_idx on
files (parent_id);

create index if not exists starred_updated_at_idx on
files (starred,
updated_at desc);

create index if not exists status_idx on files (status);

create index if not exists user_id_idx on files (user_id);