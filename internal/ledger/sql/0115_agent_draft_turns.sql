-- The conversation's turns (NT-018 phase 3, contract 7).
--
-- A turn is one message an author sends and the model's answer. It carries an
-- idempotency key, at most one is in progress per thread, and it is paid for
-- in advance: the worst case is reserved before the model is called, with a
-- lease, and the real cost settles it afterwards. A turn whose lease ran out
-- before it was stored becomes 'unknown' and is never called again; its
-- reservation is kept against the day's ceiling and never recorded as a cost.
--
-- What was said lives in run_content under owner kind 'authoring', owned by
-- the thread: the row keeps references and figures only.
--
-- Only additions. 0.118.0 does not read these columns or this table, so a
-- rollback leaves them unread. No rollback floor.
create table agent_draft_turns (
    turn_id         uuid        primary key default gen_random_uuid(),
    thread_id       uuid        not null references agent_draft_threads (thread_id),
    idempotency_key text        not null,
    seq             integer     not null,
    draft_id        uuid        not null,
    agent_id        text        not null,
    author          text        not null,
    -- The draft's ETag the model read; every proposal of the turn names it.
    draft_revision  text        not null,
    state           text        not null
        check (state in ('in_progress', 'done', 'failed', 'unknown')),
    reserved_micros bigint      not null check (reserved_micros >= 0),
    cost_micros     bigint      not null default 0 check (cost_micros >= 0),
    lease_until     timestamptz not null,
    message_ref     text        not null,
    reply_ref       text        not null default '',
    -- The author marked the message as pasted: informs the proposal's card,
    -- fences nothing.
    pasted          boolean     not null,
    created_at      timestamptz not null,
    settled_at      timestamptz,
    unique (thread_id, idempotency_key),
    unique (thread_id, seq)
);

-- At most one turn in progress per thread.
create unique index agent_draft_turns_one_in_progress
    on agent_draft_turns (thread_id) where state = 'in_progress';

-- The day's reservations, read against the ceiling.
create index agent_draft_turns_created_idx on agent_draft_turns (created_at);

-- The turn a proposal came from. Empty for none.
alter table agent_draft_proposals add column turn_id uuid references agent_draft_turns (turn_id);
