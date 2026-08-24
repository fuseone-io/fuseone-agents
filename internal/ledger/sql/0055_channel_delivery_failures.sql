-- Durable channel failures are the source for the operational cockpit.
--
-- Success already lives in channel_deliveries. Failure needs a sister table
-- because a missing Slack scope or an archived channel is not a run step and
-- does not belong in the ledger. The reporter writes it while it still knows
-- the run scope; the worker-level joined error no longer does.
--
-- Aggregated by run/event/conversation/code rather than one row per retry, so
-- a missing scope in fifty conversations says "fifty conversations affected"
-- without writing that same fact every sweep forever.
create table if not exists channel_delivery_failures (
    run_id       text        not null,
    event        text        not null,
    channel      text        not null,
    conversation text        not null,
    code         text        not null,
    company_id   text        not null,
    area_id      text        not null,
    agent_id     text        not null,
    attempts     bigint      not null default 1,
    first_seen   timestamptz not null default now(),
    last_seen    timestamptz not null default now(),

    primary key (run_id, event, channel, conversation, code)
);

create index if not exists channel_delivery_failures_scope_last_seen_idx
    on channel_delivery_failures (company_id, area_id, last_seen desc, code);

create index if not exists channel_delivery_failures_last_seen_idx
    on channel_delivery_failures (last_seen desc, code);
