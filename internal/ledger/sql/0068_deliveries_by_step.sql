-- A delivery names the question it was about, not only the run.
--
-- The sentinel that retires a run from the announcement sweep was keyed by the
-- run and the event, so a run that parked a second time was never announced
-- again: an agent asking for two tools asks for two approvals, and the second
-- one reached nobody. AtSeq was already carried in the report and drawn onto
-- the button; it was part of no key at all.
--
-- Zero for everything that is not a park, and for every row written before
-- this. A finished run has one ending and a failed run has one failure, so
-- their sentinels are unchanged.
alter table channel_deliveries
    add column if not exists at_seq bigint not null default 0;

alter table channel_deliveries drop constraint if exists channel_deliveries_pkey;
alter table channel_deliveries
    add primary key (run_id, event, channel, conversation, at_seq);

-- The same for the failures, which are keyed the same way and read beside
-- them. A conversation that could not be reached about the second park is a
-- different fact from one that could not be reached about the first.
alter table channel_delivery_failures
    add column if not exists at_seq bigint not null default 0;

alter table channel_delivery_failures drop constraint if exists channel_delivery_failures_pkey;
alter table channel_delivery_failures
    add primary key (run_id, event, channel, conversation, at_seq, code);
