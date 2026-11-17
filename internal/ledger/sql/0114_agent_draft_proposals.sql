-- The workbench's conversation: one thread per author over the shared draft,
-- and the operations the model proposes in it (NT-018 phase 3, contract 3).
--
-- The model only proposes. A proposal is bound to its author, its thread, its
-- draft and the draft's ETag the model read; only a person accepting it, by
-- its id, applies it, and a proposal made against an older draft is refused
-- (412). Its body lives in run_content under owner kind 'authoring', owned by
-- the thread, so the installation's retention and erasing the thread reach it.
--
-- Only additions: nothing the previous binary reads changes, and 0.118.0
-- leaves these tables unread. No rollback floor.

-- One thread per author per draft. A draft discarded and opened again is
-- another draft, with threads of its own.
create table agent_draft_threads (
    thread_id  uuid        primary key default gen_random_uuid(),
    draft_id   uuid        not null,
    agent_id   text        not null,
    author     text        not null,
    created_at timestamptz not null,
    unique (draft_id, author)
);

-- What the model proposed. Ids, references and digests only: immutable rows
-- hold no free text, so the operation and the reason a proposal is invalid
-- live in the body.
create table agent_draft_proposals (
    proposal_id    uuid        primary key default gen_random_uuid(),
    thread_id      uuid        not null references agent_draft_threads (thread_id),
    seq            integer     not null,
    draft_id       uuid        not null,
    agent_id       text        not null,
    author         text        not null,
    -- The draft's ETag the model read: accepting it against any other is a 412.
    draft_revision text        not null,
    -- The one discriminator left once the body is erased, so only the kinds
    -- there are (draft.Operations).
    kind           text        not null check (kind in (
        'set_identity', 'set_standing_approvals', 'set_model', 'set_fallback',
        'set_cache_prefix_ttl', 'set_reply_locale', 'add_tool', 'remove_tool',
        'set_budget', 'set_triggers', 'set_memory_learning', 'set_approval_delivery',
        'set_emits', 'add_step', 'edit_step', 'remove_step',
        'insert_block', 'replace_block', 'remove_block', 'move_block')),
    digest         text        not null,
    body_ref       text        not null,
    -- Validated when proposed, as an operation by proposal: an invalid one is
    -- shown crossed out and can never be accepted.
    valid          boolean     not null,
    created_at     timestamptz not null,
    unique (thread_id, seq)
);

create index agent_draft_proposals_agent_idx on agent_draft_proposals (agent_id, draft_id);

-- What a person decided about a proposal: once, and for good.
create table agent_draft_proposal_decisions (
    proposal_id uuid        primary key references agent_draft_proposals (proposal_id),
    decision    text        not null check (decision in ('accepted', 'rejected')),
    -- The draft's ETag the acceptance left; empty for a rejection.
    revision    text        not null default '',
    by_user     text        not null,
    at          timestamptz not null
);

create or replace function agent_draft_proposals_append_only() returns trigger
language plpgsql as $$
begin
    raise exception '% is append-only: % rejected', tg_table_name, lower(tg_op)
        using errcode = 'restrict_violation';
end;
$$;

create trigger agent_draft_proposals_no_update
    before update or delete on agent_draft_proposals
    for each row execute function agent_draft_proposals_append_only();

create trigger agent_draft_proposal_decisions_no_update
    before update or delete on agent_draft_proposal_decisions
    for each row execute function agent_draft_proposals_append_only();
