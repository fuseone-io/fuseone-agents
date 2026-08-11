-- 0001_ledger.sql
-- The ledger: the single source of truth for run state, audit trail, cost
-- rollups, replay and the regression corpus (PRD AU-02).

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

    prev_hash    bytea,
    hash         bytea       not null,

    -- Single writer per run (PRD NF-15). Two goroutines racing for the same
    -- sequence: the second one gets a unique violation and loses. This is the
    -- enforcement point, not a convention the application is trusted to keep.
    primary key (run_id, seq),

    constraint run_steps_seq_positive check (seq >= 1),
    constraint run_steps_first_has_no_parent check (
        (seq = 1 and prev_hash is null) or (seq > 1 and prev_hash is not null)
    )
);

-- Idempotency (Gate check 6). A retry or a resume that replays the same tool
-- call with the same arguments collides here instead of billing twice.
create unique index run_steps_idem_key_uniq
    on run_steps (idem_key)
    where idem_key <> '';

-- Cost rollups walk the scope hierarchy: company -> area -> agent (PRD FO-07).
create index run_steps_scope_at_idx on run_steps (company_id, area_id, at desc);
create index run_steps_agent_at_idx on run_steps (agent_id, at desc);

-- Approval inbox and parked-run sweeps read by kind.
create index run_steps_kind_at_idx on run_steps (kind, at desc)
    where kind in ('approval_requested', 'parked', 'failed');

-- Append-only, enforced by the database (PRD AU-01, engineering rule 2).
-- Application code cannot be the only guard: a migration, a console session or
-- a well-meaning cleanup script would silently break the hash chain and every
-- projection built on it.
create or replace function run_steps_append_only() returns trigger
language plpgsql as $$
begin
    raise exception 'run_steps is append-only: % rejected on run %',
        tg_op, coalesce(old.run_id, new.run_id)
        using errcode = 'restrict_violation';
end;
$$;

create trigger run_steps_no_update
    before update on run_steps
    for each row execute function run_steps_append_only();

create trigger run_steps_no_delete
    before delete on run_steps
    for each row execute function run_steps_append_only();
