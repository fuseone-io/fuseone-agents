-- The ledger, split by month.
--
-- run_steps is the one table that only ever grows: nothing updates it and
-- nothing deletes from it, by design and by trigger. Measured at 824 bytes a
-- step, a modest installation writes tens of gigabytes a year, and the cost
-- that actually bites is not the reading — indexes handle that — but the
-- maintenance: vacuum, reindex, and the eventual need to move a year nobody
-- reads onto slower storage. Those are partition operations.
--
-- WHAT THE KEY IS, AND WHY IT IS NOT `at`
--
-- The obvious key is the step's own timestamp. It is wrong here, and quietly.
-- Postgres requires the partition key inside every unique constraint, so with
-- `at` as the key:
--
--   * the primary key that enforces one writer per run (NF-15) becomes
--     (run_id, seq, at), which is unique only within a partition; and
--   * the unique index that enforces idempotency (Gate check 6) has the same
--     problem.
--
-- Neither matters until a run is still open when the month turns — a run
-- parked over a weekend waiting for a person, a compensation that took days —
-- and then its steps sit in two partitions where neither constraint can see
-- the other half. Two writers could each claim sequence 12; the same effect
-- could be billed twice. Both are silent.
--
-- So the key is `opened_at`: the time the run started, carried on every one of
-- its steps and never changing. A run's steps therefore always share a
-- partition whatever its length, and (run_id, seq, opened_at) is the primary
-- key it always was.
--
-- WHAT THIS STILL GIVES UP
--
-- Two runs sharing a run_id and opened in different months would no longer
-- collide. The identifier embeds a millisecond timestamp, so it cannot happen
-- — but it is now prevented by how identifiers are made rather than by the
-- database, which is a weaker guarantee than the one above it, and saying so
-- here is the point.
--
-- run_content is the next candidate and is deliberately not done here: it
-- holds a tombstone per erasure that has to outlive the bytes, so dropping a
-- partition of it is a different decision from dropping a month of steps.

alter table run_steps rename to run_steps_legacy;
alter table run_steps_legacy rename constraint run_steps_pkey to run_steps_legacy_pkey;
drop index run_steps_idem_key_uniq;
drop index run_steps_scope_at_idx;
drop index run_steps_agent_at_idx;
drop index run_steps_kind_at_idx;
drop index run_steps_trail_idx;
drop index run_steps_agreement_idx;
drop index run_steps_decided_idx;

create table run_steps (
    run_id       text        not null,
    seq          bigint      not null,
    kind         text        not null,

    company_id   text        not null,
    area_id      text        not null,

    agent_id     text        not null,
    version_id   text        not null,
    on_behalf_of text        not null default '',

    payload      jsonb       not null default '{}'::jsonb,
    labels       text[]      not null default '{}',

    input_tokens       bigint not null default 0,
    output_tokens      bigint not null default 0,
    cache_read_tokens  bigint not null default 0,
    cache_write_tokens bigint not null default 0,
    cost_micros        bigint not null default 0,

    idem_key     text        not null default '',
    policy_hash  text        not null default '',

    at           timestamptz not null,
    -- When the run this step belongs to started. The same value on every step
    -- of a run, which is what keeps a run whole inside one partition.
    opened_at    timestamptz not null,

    prev_hash    bytea,
    hash         bytea       not null,

    primary key (run_id, seq, opened_at),

    constraint run_steps_seq_positive check (seq >= 1),
    constraint run_steps_first_has_no_parent check (
        (seq = 1 and prev_hash is null) or (seq > 1 and prev_hash is not null)
    )
) partition by range (opened_at);

-- The months the existing ledger covers, plus a year either side of today, so
-- a fresh installation and a migrated one both start with somewhere to write.
do $$
declare
    from_month date;
    to_month   date;
    m          date;
begin
    select coalesce(date_trunc('month', min(at))::date,
                    date_trunc('month', now())::date),
           coalesce(date_trunc('month', max(at))::date,
                    date_trunc('month', now())::date)
      into from_month, to_month
      from run_steps_legacy;

    from_month := least(from_month, (date_trunc('month', now()) - interval '1 year')::date);
    -- Capped, not followed. One step stamped by a machine whose clock is years
    -- fast would otherwise materialise a partition a month all the way there;
    -- past the cap it lands in the default partition, which is what that is
    -- for.
    to_month := least(greatest(to_month, (date_trunc('month', now()) + interval '1 year')::date),
                      (date_trunc('month', now()) + interval '1 year')::date);

    m := from_month;
    while m <= to_month loop
        execute format(
            'create table %I partition of run_steps for values from (%L) to (%L)',
            'run_steps_' || to_char(m, 'YYYY_MM'), m, (m + interval '1 month')::date);
        m := (m + interval '1 month')::date;
    end loop;
end $$;

insert into run_steps (
    run_id, seq, kind, company_id, area_id, agent_id, version_id, on_behalf_of,
    payload, labels, input_tokens, output_tokens, cache_read_tokens,
    cache_write_tokens, cost_micros, idem_key, policy_hash, at, opened_at,
    prev_hash, hash)
select s.run_id, s.seq, s.kind, s.company_id, s.area_id, s.agent_id,
       s.version_id, s.on_behalf_of, s.payload, s.labels,
       s.input_tokens, s.output_tokens, s.cache_read_tokens,
       s.cache_write_tokens, s.cost_micros, s.idem_key, s.policy_hash,
       s.at, opened.first_at, s.prev_hash, s.hash
from run_steps_legacy s
join (select run_id, min(at) as first_at from run_steps_legacy group by run_id) opened
  on opened.run_id = s.run_id;

drop table run_steps_legacy;

-- A month nobody thought to create still has to be recordable. A ledger that
-- refused an append because of its own housekeeping would be a worse failure
-- than a large table, so everything outside the declared months lands here and
-- is as correct as anywhere else — every unique constraint contains the
-- partition key, so rows that must not collide cannot be in different
-- partitions. What it costs is archival: a default partition cannot be
-- detached as a month. cmd/agentd keeps months ahead of the clock so it stays
-- empty, and `agentd verify` reports it when it does not.
create table run_steps_default partition of run_steps default;

-- Idempotency (Gate check 6). The key already carries the run identifier, so
-- leading with the partition key costs nothing: two calls that must collide
-- belong to one run and therefore to one partition.
create unique index run_steps_idem_key_uniq
    on run_steps (opened_at, idem_key)
    where idem_key <> '';

create index run_steps_scope_at_idx on run_steps (company_id, area_id, at desc);
create index run_steps_agent_at_idx on run_steps (agent_id, at desc);
create index run_steps_kind_at_idx on run_steps (kind, at desc)
    where kind in ('approval_requested', 'parked', 'failed');
create index run_steps_trail_idx on run_steps (at desc, run_id desc, seq desc)
    where kind in ('gate_decided', 'approval_decided');
create index run_steps_agreement_idx on run_steps (agent_id, at desc)
    where kind = 'approval_decided';
create index run_steps_decided_idx on run_steps (at desc, run_id desc, seq desc)
    where kind = 'gate_decided';

-- Append-only, still enforced by the database (AU-01, engineering rule 2).
-- Declared on the parent: Postgres propagates a row trigger to every partition
-- there is and every one added later, which is the only version of this that
-- cannot be forgotten when a month is created.
create trigger run_steps_no_update
    before update on run_steps
    for each row execute function run_steps_append_only();

create trigger run_steps_no_delete
    before delete on run_steps
    for each row execute function run_steps_append_only();

-- Making the month ahead. Called by the platform on a timer and by hand when
-- somebody is restoring; either way it is safe to call twice.
create or replace function run_steps_month(month date) returns text
language plpgsql as $$
declare
    name text := 'run_steps_' || to_char(month, 'YYYY_MM');
begin
    if to_regclass(name) is not null then
        return name;
    end if;
    execute format(
        'create table %I partition of run_steps for values from (%L) to (%L)',
        name, date_trunc('month', month)::date,
        (date_trunc('month', month) + interval '1 month')::date);
    return name;
end;
$$;
