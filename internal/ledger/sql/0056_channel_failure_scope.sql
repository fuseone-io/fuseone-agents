-- A failure to read channel configuration happens before the reporter knows
-- which conversations would have received the message. Counting the empty
-- channel/conversation placeholder as one conversation understates the blast
-- radius with confidence, so the aggregate keeps that shape explicit.
alter table channel_delivery_failures
    add column if not exists scope_wide boolean not null default false;

update channel_delivery_failures
set scope_wide = true
where channel = '' and conversation = '';

-- Retention sweeps channel_inbox by arrival time across settled rows. The
-- existing inbox indexes are partial and serve claiming/opened/refused queues;
-- this one serves deletion of old operational records after open debts have
-- been answered.
create index if not exists channel_inbox_at_idx
    on channel_inbox (at);
