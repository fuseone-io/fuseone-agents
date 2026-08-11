-- Where each schedule's next moment is kept.
--
-- One row per (agent, schedule) rather than per agent: an agent can declare
-- more than one, and a schedule that changes between versions is a new row
-- rather than a silent rewrite of the old one's next moment.
--
-- Nothing here is a lock. Two workers reading the same due row both open the
-- same run, because the run's idempotency key is derived from the moment and
-- the ledger accepts one of them. This table only remembers when to look.
create table if not exists trigger_schedules (
    agent_id     text        not null,
    schedule     text        not null,
    company_id   text        not null default '',
    area_id      text        not null default '',
    next_fire_at timestamptz not null,
    updated_at   timestamptz not null default now(),

    primary key (agent_id, schedule)
);

-- The tick's only query: what is due.
create index if not exists trigger_schedules_due_idx
    on trigger_schedules (next_fire_at);
