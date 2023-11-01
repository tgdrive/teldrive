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
drop procedure if exists teldrive.delete_files;