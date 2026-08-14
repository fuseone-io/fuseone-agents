-- One table for areas, and it is the one somebody writes to.
--
-- There were two. `areas` (0004) carried a foreign key to companies and was
-- written by exactly one statement in the whole platform — the bootstrap.
-- `scopes` (0013) carried no foreign key and is what the console reads, the
-- scope store writes and every screen lists.
--
-- So the integrity lived on the table nobody used, and the table everybody used
-- accepted an area in a company that does not exist. That is not theoretical:
-- registering an area in an unknown company saved cleanly and then vanished
-- from every listing, because listing is filtered by the scopes a caller holds
-- and nobody holds a company that was never created.
--
-- The permission check added alongside this is the guard at the door. This is
-- the guard in the wall behind it, and a platform whose whole argument is a
-- record worth trusting should not have a governance table that admits rows
-- referring to nothing.

-- Whatever the bootstrap wrote, kept.
insert into scopes (company_id, area_id, label, created_by)
select a.company_id, a.area_id, coalesce(nullif(a.name, ''), a.area_id), 'bootstrap'
from areas a
on conflict (company_id, area_id) do nothing;

-- Every company an existing area refers to, so the key below can be added
-- without discarding anybody's data. An installation that has areas in a
-- company nobody registered gets that company registered rather than losing
-- the areas.
insert into companies (company_id, name)
select distinct s.company_id, s.company_id
from scopes s
where s.company_id <> ''
  and not exists (select 1 from companies c where c.company_id = s.company_id)
on conflict (company_id) do nothing;

alter table scopes
    add constraint scopes_company_fk
    foreign key (company_id) references companies(company_id);

drop table areas;
