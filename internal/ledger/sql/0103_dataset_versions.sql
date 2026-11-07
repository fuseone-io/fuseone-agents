-- What a battery was checked against (NT-015, phase 1).
--
-- A corpus has no history: recording replaces a case and deleting removes one, so a
-- past result cannot be tied to what it ran. A version is the corpus as it stood,
-- written with the change that made it.
--
-- This migration changes no behaviour. It adds the tables and a switch that is off,
-- and every writer reads the switch inside its transaction. A later migration, after
-- a release in between, turns it on: a version can only be trusted if every write
-- produced one, and during a rolling update an older pod still writes.
create table dataset_versioning (
    only_row     boolean     primary key default true check (only_row),
    active       boolean     not null default false,
    activated_at timestamptz
);

insert into dataset_versioning (only_row) values (true);

-- An ordinary row. A name can change, which is why it stays out of what is immutable.
create table datasets (
    dataset_id text        primary key,
    agent_id   text        not null,
    name       text        not null,
    company_id text        not null default '',
    area_id    text        not null default '',
    created_at timestamptz not null default now(),

    unique (agent_id, name)
);

create table dataset_versions (
    dataset_id text        not null references datasets (dataset_id),
    version    bigint      not null check (version >= 1),
    digest     text        not null,
    created_by text        not null default '',
    created_at timestamptz not null default now(),

    primary key (dataset_id, version)
);

-- Ids, references and digests, and nothing somebody typed: this row outlives every
-- erasure, so anything readable in it would too. The bytes behind these references
-- are content, under retention, and a tombstone keeps its digest, so erasing does not
-- change what a version is.
create table dataset_version_cases (
    dataset_id          text   not null,
    version             bigint not null,
    case_id             text   not null,
    input_ref           text   not null,
    input_digest        text   not null,
    expectations_ref    text   not null,
    expectations_digest text   not null,
    from_run            text   not null default '',

    primary key (dataset_id, version, case_id),
    foreign key (dataset_id, version) references dataset_versions (dataset_id, version)
);

-- Erasure starts from a copy, or from a run somebody named.
create index dataset_version_cases_input_idx on dataset_version_cases (input_ref);
create index dataset_version_cases_from_run_idx on dataset_version_cases (from_run)
    where from_run <> '';

create or replace function dataset_versions_append_only() returns trigger
language plpgsql as $$
begin
    raise exception '% is append-only: % rejected', tg_table_name, tg_op
        using errcode = 'restrict_violation';
end;
$$;

create trigger dataset_versions_no_update before update on dataset_versions
    for each row execute function dataset_versions_append_only();
create trigger dataset_versions_no_delete before delete on dataset_versions
    for each row execute function dataset_versions_append_only();
create trigger dataset_version_cases_no_update before update on dataset_version_cases
    for each row execute function dataset_versions_append_only();
create trigger dataset_version_cases_no_delete before delete on dataset_version_cases
    for each row execute function dataset_versions_append_only();

-- Where a case's expectations live as content. Empty until activation, and the column
-- stays until the version reading it has left the rollback window.
alter table regression_cases add column expectations_ref text;

create index regression_cases_expectations_ref_idx on regression_cases (expectations_ref)
    where expectations_ref is not null;

-- What a case expected goes with its input, in every version naming it, whichever
-- binary did the erasing. A version outlives the case, so this follows the version
-- rows and not the working row.
--
-- The early return is what ends the recursion: the second update here fires this
-- trigger again, and a statement trigger fires even for no rows.
create or replace function clear_expectations_of_erased_copies() returns trigger
language plpgsql as $$
begin
    if not exists (select 1 from changed where erased_at is not null) then
        return null;
    end if;

    update regression_cases c
    set expectations = '[]'::jsonb
    from changed
    where (c.input_ref = changed.ref or c.expectations_ref = changed.ref)
      and changed.erased_at is not null
      and c.expectations <> '[]'::jsonb;

    update run_content held
    set bytes = null, erased_at = now(), erased_reason = changed.erased_reason
    from changed
    join dataset_version_cases v on v.input_ref = changed.ref
    where changed.erased_at is not null
      and held.ref = v.expectations_ref
      and held.erased_at is null;

    return null;
end;
$$;
