-- Candidate versions (NT-018 phase 2, release 2a).
--
-- A candidate is a draft sealed into the bytes a publication would write: a
-- version id computed as the publication computes it, kept outside
-- agent_specs and therefore invisible to every reader of "the current
-- version". This release only teaches readers the shape. Nothing writes a
-- seal until release 2b, which must wait for this one on every replica.
--
-- A seal is a fact about bytes, like the claim it names: insert-only. The
-- bytes themselves live in run_content under the seal's id, so erasure and
-- retention reach them while this row keeps only references.
create table agent_seals (
    seal_id        uuid        not null primary key,
    agent_id       text        not null,
    version_id     text        not null,
    -- The scope the draft was sealed in. A candidate runs there even if the
    -- agent moves afterwards.
    company_id     text        not null,
    area_id        text        not null,
    -- The sealed definition in run_content; the claim holds its digest.
    content_ref    text        not null,
    -- No foreign key: a discarded draft leaves its seals behind.
    draft_id       uuid        not null,
    draft_revision text        not null,
    sealed_by      text        not null,
    sealed_at      timestamptz not null default now(),

    foreign key (agent_id, version_id)
        references agent_version_digests (agent_id, version_id)
);
create index agent_seals_version_idx on agent_seals (agent_id, version_id);

-- Where each sealed value came from, copied from the draft's source map when
-- the seal was taken. A NULL trail_seq is a value whose source could not be
-- proved; erasure of any thread of that draft reaches it.
create table agent_seal_sources (
    seal_id   uuid    not null references agent_seals (seal_id),
    part      text    not null,
    trail_seq integer,

    primary key (seal_id, part)
);

create function agent_seals_are_permanent() returns trigger
language plpgsql as $$
begin
    raise exception '% is insert-only: % rejected (NT-018)', tg_table_name, lower(tg_op)
        using errcode = 'restrict_violation';
end;
$$;

create trigger agent_seals_permanent
    before update or delete on agent_seals
    for each row execute function agent_seals_are_permanent();
create trigger agent_seal_sources_permanent
    before update or delete on agent_seal_sources
    for each row execute function agent_seals_are_permanent();

-- A run opened on a candidate, and the authoring thread it came from, as its
-- run_started step says. Erasing a seal or a thread follows these to the
-- runs derived from it. Adding a nullable column without a default does not
-- rewrite the table; the indexes are built concurrently by 0119.
alter table runs
    add column seal_id          uuid,
    add column authoring_thread uuid;
