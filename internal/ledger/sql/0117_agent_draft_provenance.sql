-- Which accepted operation wrote each value still present in a workbench
-- draft (NT-018 phase 2, delivery P).
--
-- The trail remains append-only.  Its provenance lives beside it so an
-- accepted proposal can be followed back to its thread without putting free
-- text in an immutable row.  The mutable source map follows the mutable
-- draft: removing or replacing a value removes or replaces its source.
create table agent_draft_trail_provenance (
    draft_id   uuid    not null,
    trail_seq  integer not null,
    proposal_id uuid references agent_draft_proposals (proposal_id),
    -- Semantic part names only: block ids, tool ids and hashes of step names.
    parts      text[]  not null default '{}',

    primary key (draft_id, trail_seq),
    foreign key (draft_id, trail_seq)
        references agent_draft_trail (draft_id, seq)
);

create table agent_draft_sources (
    draft_id  uuid    not null references agent_drafts (draft_id) on delete cascade,
    part      text    not null,
    trail_seq integer not null,

    primary key (draft_id, part),
    foreign key (draft_id, trail_seq)
        references agent_draft_trail_provenance (draft_id, trail_seq)
);

-- A source map is useful only for the exact draft state it describes.  A
-- replica from before this migration changes revision and appends a trail row
-- without moving this stamp, which makes every conversation-capable value
-- conservative/unknown until a new writer repairs the map.
alter table agent_drafts
    add column sources_revision text not null default '',
    add column sources_seq integer not null default 0;

-- Phase 4 will write this.  Delivery P only makes subject erasure able to
-- follow the reference when it exists.
alter table agent_draft_turns
    add column referenced_run text,
    -- Distinct from erased: that flag means the whole thread was erased and
    -- is what the legacy repairer follows. Subject erasure reaches one turn.
    add column subject_erased boolean not null default false;
create index agent_draft_turns_referenced_run_idx
    on agent_draft_turns (referenced_run) where referenced_run is not null;

-- The repairer only needs to revisit thread erasures performed by an older
-- binary once. This marker contains no authored content.
alter table agent_draft_threads add column erasure_reconciled boolean not null default false;

create or replace function agent_draft_trail_provenance_append_only() returns trigger
language plpgsql as $$
begin
    raise exception 'agent_draft_trail_provenance is append-only: % rejected on % #%',
        lower(tg_op), old.draft_id, old.trail_seq
        using errcode = 'restrict_violation';
end;
$$;

create trigger agent_draft_trail_provenance_no_update
    before update or delete on agent_draft_trail_provenance
    for each row execute function agent_draft_trail_provenance_append_only();
