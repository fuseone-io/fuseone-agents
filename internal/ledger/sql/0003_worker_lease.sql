-- 0003_worker_lease.sql
-- Coordination columns so a pool of workers can share the run queue.
--
-- The lease is deliberately not a held transaction. Advancing a run makes a
-- model call and a tool call, which take seconds; holding a row lock across
-- that would pin a database connection per in-flight agent and turn a slow
-- upstream into a database outage.

alter table runs
    -- attempts counts consecutive failures, and resets on any turn that makes
    -- progress. It drives the backoff and the parking threshold (PRD NF-14).
    add column attempts int not null default 0,

    -- next_attempt_at is when this run becomes claimable again. Backoff writes
    -- the future into it rather than the worker sleeping, so a restart does
    -- not forget a pending delay.
    add column next_attempt_at timestamptz not null default now(),

    -- leased_until fences a claim. A worker that dies stops renewing, the
    -- lease expires, and another worker picks the run up — no heartbeat
    -- protocol and no leader election.
    add column leased_until timestamptz,
    add column lease_owner  text,

    -- last_error is what the console shows next to a parked run, so the owner
    -- knows whether to retry or fix something upstream.
    add column last_error text;

drop index runs_resumable_idx;

-- The claim query, and the only index it needs. Partial so it stays the size
-- of the working set rather than the size of history: finished runs and runs
-- awaiting a human never appear in it.
create index runs_claimable_idx
    on runs (next_attempt_at)
    where phase in ('running', 'awaiting_tool');
