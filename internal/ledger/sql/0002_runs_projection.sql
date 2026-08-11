-- 0002_runs_projection.sql
-- A materialised projection of the ledger, maintained inside the same
-- transaction as the append that changes it.
--
-- Listing runs by folding every step per request is honest for a development
-- store and dies at real volume: a console page would replay the entire
-- history of every run it displays. The ledger stays the source of truth —
-- this table can be dropped and rebuilt from it at any time.

create table runs (
    run_id      text        not null primary key,

    company_id  text        not null,
    area_id     text        not null,
    agent_id    text        not null,
    version_id  text        not null,
    on_behalf_of text       not null default '',

    phase       text        not null,
    last_seq    bigint      not null,

    cost_micros        bigint not null default 0,
    total_tokens       bigint not null default 0,
    reserved_micros    bigint not null default 0,
    tool_calls         bigint not null default 0,

    labels      text[]      not null default '{}',

    -- Denormalised so the approval inbox is one indexed scan rather than a
    -- fold of every suspended run.
    pending_tool     text,
    pending_rule     text,
    pending_reason   text,
    pending_at_seq   bigint,

    started_at  timestamptz not null,
    ended_at    timestamptz,
    updated_at  timestamptz not null
);

-- The console's default view: newest first within a scope.
create index runs_scope_started_idx on runs (company_id, area_id, started_at desc);
create index runs_agent_started_idx on runs (agent_id, started_at desc);
create index runs_phase_idx on runs (phase, started_at desc);

-- The approval inbox, oldest first: partial so it stays small however many
-- finished runs accumulate.
create index runs_pending_approval_idx on runs (company_id, area_id, pending_at_seq)
    where pending_tool is not null;

-- Workers claim resumable runs from here with FOR UPDATE SKIP LOCKED.
create index runs_resumable_idx on runs (updated_at)
    where phase in ('running', 'awaiting_tool', 'parked');

