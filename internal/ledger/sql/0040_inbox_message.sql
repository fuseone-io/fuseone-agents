-- Which message asked, as the channel names it.
--
-- The inbox kept the delivery id and used it for both jobs. In Slack those are
-- two different things: `event_id` names a delivery and repeats on every retry
-- of it, while `event.ts` names the message and is what a thread is keyed by.
-- Sealed as the message, the origin pointed at a retry rather than at what
-- somebody typed — and the check for "did this ask start its own thread"
-- compared `thread` to a delivery id and was never true.
--
-- One id for idempotency and another for identity. Conflating them worked in a
-- fixture where both were the same string, which is how it survived review.
alter table channel_inbox
    add column if not exists message text not null default '';
