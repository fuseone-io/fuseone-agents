-- What each planning call cost, projected for aggregation.
--
-- One row per `planned` step rather than per run, because a step may name its
-- own model: aggregating by run would attribute a cheap step's spend to the
-- agent's default model, and a financial chart that is wrong in a direction
-- nobody checks is worse than no chart.
--
-- The ledger stays the source of truth. This is a projection written by a
-- sweep rather than by the appender, so a mistake here is rebuilt by
-- reprocessing instead of by rewriting a chain that must not be rewritten.

create table if not exists planning_spend (
    run_id text not null,
    seq bigint not null,

    -- The pair the call was actually made against, recorded on the step.
    provider text not null default '',
    model text not null default '',

    agent_id text not null,
    company_id text not null,
    area_id text not null,
    -- Kept as a date rather than derived on read: every aggregate groups by it,
    -- and to_char over a scan is the difference between a page and a wait.
    day date not null,

    cost_micros bigint not null default 0,
    input_tokens bigint not null default 0,
    output_tokens bigint not null default 0,
    cache_read_tokens bigint not null default 0,
    cache_write_tokens bigint not null default 0,

    -- Whether the figure above is a rate somebody set. An aggregate that sums
    -- unpriced calls into a total reports confidence it does not have.
    price_status text not null default '',

    primary key (run_id, seq)
);

create index if not exists planning_spend_day_model_idx
    on planning_spend (day, provider, model);

create index if not exists planning_spend_day_agent_idx
    on planning_spend (day, agent_id);

-- What the sweep reads. The cursor walks planning steps in append order, and
-- no existing index covers them: run_steps_kind_at_idx names the three kinds a
-- trail filter asks for and planned is not one, while the trail index covers
-- gate and approval decisions. Without this the sweep scans the chain on every
-- pass to find the rows it already knows how to skip.
--
-- Ascending, unlike the trail indexes: a cursor resumes forward from where it
-- stopped, where a screen pages backward from now.
create index if not exists run_steps_planned_cursor_idx
    on run_steps (at, run_id, seq)
    where kind = 'planned';

create table if not exists planning_spend_cursor (
    id boolean primary key default true check (id),
    scanned_at timestamptz not null,
    scanned_run_id text not null default '',
    scanned_seq bigint not null default 0
);

-- From deployment forward. Steps recorded before this carry no provider or
-- model — the field existed and nothing wrote it — so backfilling would mean
-- attributing spend to a model the step never named.
insert into planning_spend_cursor (id, scanned_at)
values (true, now())
on conflict (id) do nothing;
