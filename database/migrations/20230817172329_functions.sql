-- +goose Up

-- +goose StatementBegin
create procedure teldrive.update_size() language plpgsql as $$
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
-- +goose StatementEnd


-- +goose StatementBegin
create function teldrive.update_folder(folder_id text,
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
-- +goose StatementEnd

-- +goose StatementBegin
create procedure teldrive.delete_files(in file_ids text[],
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
-- +goose StatementEnd


-- +goose Down
drop procedure if exists teldrive.update_size;
drop function if exists teldrive.update_folder;
drop procedure if exists teldrive.delete_files;