-- New Gate blocks are operationally interesting the first time they appear.
--
-- The ledger remains the source of truth. This table is only a projection that
-- remembers which low-cardinality shapes have already been announced, so the
-- worker can tell people once without folding the append-only chain on every
-- pass.

create table if not exists gate_refusal_forms (
    company_id text not null,
    area_id text not null,

    -- policy_code when an authored policy produced the block; otherwise the
    -- Gate rule name. Kept as one key so editing a policy's prose does not
    -- turn the same rule into a new alert.
    rule_key text not null,
    rule text not null,
    policy_code text not null default '',

    tool text not null,
    effect smallint not null,
    verdict smallint not null,

    first_run_id text not null,
    first_seq bigint not null,
    first_agent_id text not null,
    first_seen_at timestamptz not null,

    last_run_id text not null,
    last_seq bigint not null,
    last_agent_id text not null,
    last_seen_at timestamptz not null,

    announced_at timestamptz,
    lease_owner text not null default '',
    lease_until timestamptz,

    primary key (company_id, area_id, rule_key, tool, effect, verdict)
);

create index if not exists gate_refusal_forms_pending_idx
    on gate_refusal_forms (first_seen_at)
    where announced_at is null;

create table if not exists gate_refusal_alert_cursor (
    id boolean primary key default true check (id),
    scanned_at timestamptz not null,
    scanned_run_id text not null default '',
    scanned_seq bigint not null default 0
);

-- Start at deployment time. Replaying every historical block into Slack would
-- be a migration surprise, not an alert.
insert into gate_refusal_alert_cursor (id, scanned_at)
values (true, now())
on conflict (id) do nothing;
