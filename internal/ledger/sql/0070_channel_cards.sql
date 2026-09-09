-- A delivery becomes a card when it offers an answer.
--
-- Three columns rather than a table: the row already records the message, the
-- connection and the vendor's reference, and a second table would need every
-- one of them again plus a rule for keeping the two in step.

-- Where the vendor put it, when that is not where it was sent. A direct
-- message is addressed to a person and lands in a conversation Slack names
-- itself, and chat.update takes the second. Empty means they are the same.
alter table channel_deliveries
    add column if not exists placed_in text not null default '';

-- When the card stopped offering an answer. Written on a successful edit and
-- also on a refusal nothing can fix: a message somebody deleted cannot be
-- closed, and retrying it every sweep for ever is the same silence with a cost.
alter table channel_deliveries
    add column if not exists closed_at timestamptz;

-- The sweep asks for cards still offering an answer, which is a small and
-- shrinking set inside a table that only grows.
create index if not exists channel_deliveries_open_cards_idx
    on channel_deliveries (run_id)
    where closed_at is null and at_seq > 0;
