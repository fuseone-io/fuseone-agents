-- Published agent specifications.
--
-- They lived only on the worker's disk, which meant the API could not answer
-- "which agents exist" and two processes disagreed about what was published.
-- A specification is installation state: runs are pinned to a version, and an
-- auditor reading a two-year-old run needs the exact text it ran under.
--
-- The version is the digest of the file's bytes, so a row is immutable by
-- construction: the same version is the same content, and different content is
-- a different version. Publishing is insert-or-nothing for that reason — there
-- is no way to edit a published version in place (PRD DE-08).
create table agent_specs (
    agent_id     text        not null,
    version_id   text        not null,

    company_id   text        not null default '',
    area_id      text        not null,

    name         text        not null,
    provider     text        not null,
    model        text        not null,
    effort       text        not null default '',

    tools        text[]      not null default '{}',
    budget       jsonb       not null default '{}'::jsonb,
    triggers     jsonb       not null default '[]'::jsonb,
    instructions text        not null default '',
    source       text        not null default '',

    published_by text        not null default '',
    published_at timestamptz not null default now(),

    primary key (agent_id, version_id)
);

-- A published version is a fact, not a record to be maintained.
create or replace function agent_specs_are_immutable() returns trigger as $$
begin
    raise exception 'agent_specs is insert-only: a published version cannot be changed (PRD DE-08)';
end;
$$ language plpgsql;

create trigger agent_specs_no_update before update or delete on agent_specs
    for each row execute function agent_specs_are_immutable();

create index agent_specs_recent_idx on agent_specs (agent_id, published_at desc);
create index agent_specs_scope_idx on agent_specs (company_id, area_id);
