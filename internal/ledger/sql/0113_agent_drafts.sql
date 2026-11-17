-- The workbench's draft, and the trail of what changed it (NT-018 1b).
--
-- A draft is mutable working state: not the ledger and not a version. One per
-- agent, shared by its authors, holding the whole definition in the
-- publication's own format (no body) and the instructions as blocks with
-- stable ids and their raw bytes. It changes only by operations, each
-- applied under the row's lock against the ETag the caller read.
--
-- Nothing here is read by the previous binary, and nothing a draft does writes
-- agent_specs or agent_state: a draft is invisible to publication, listing and
-- runs until the workbench publishes it, which is a later delivery. Rolling
-- back to 0.117.0 leaves these tables unread and harmless.
create table agent_drafts (
    agent_id     text        primary key,
    -- Generated here, once, and never reused: a draft discarded and opened
    -- again is another draft, with a trail and an ETag of its own.
    draft_id     uuid        not null unique default gen_random_uuid(),
    company_id   text        not null,
    area_id      text        not null,
    definition   text        not null,
    blocks       jsonb       not null,
    -- The version the draft was opened from; empty for an agent that did
    -- not exist yet.
    base_version text        not null default '',
    revision     text        not null,
    created_by   text        not null,
    created_at   timestamptz not null,
    updated_by   text        not null,
    updated_at   timestamptz not null,
    -- The workbench's draft always names a whole scope, and never the
    -- installation's.
    constraint agent_drafts_whole_scope
        check (company_id <> '' and company_id <> '*' and area_id <> '')
);

-- Every change to a draft, in order. Ids, references and digests only: the
-- rows are immutable, and an immutable row holds no free text. What a change
-- said lives in run_content under owner kind 'authoring', written in the same
-- transaction and exactly, so the installation's retention and erasure reach
-- it; the row keeps its reference and digest.
create table agent_draft_trail (
    draft_id  uuid        not null,
    seq       integer     not null,
    agent_id  text        not null,
    -- create, discard, or the operation's kind.
    action    text        not null,
    -- The SHA-256 of the change's body, or empty for a discard.
    digest    text        not null default '',
    -- The body's reference in run_content, or empty for a discard.
    body_ref  text        not null default '',
    -- control or proposal.
    origin    text        not null,
    -- The draft's ETag once the change was applied.
    revision  text        not null,
    by_user   text        not null,
    at        timestamptz not null,
    primary key (draft_id, seq)
);

create index agent_draft_trail_agent_idx on agent_draft_trail (agent_id, at);

create or replace function agent_draft_trail_append_only() returns trigger
language plpgsql as $$
begin
    raise exception 'agent_draft_trail is append-only: % rejected on % #%',
        lower(tg_op), old.draft_id, old.seq
        using errcode = 'restrict_violation';
end;
$$;

create trigger agent_draft_trail_no_update
    before update or delete on agent_draft_trail
    for each row execute function agent_draft_trail_append_only();
