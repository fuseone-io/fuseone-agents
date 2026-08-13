-- What has already been said, and where.
--
-- A run reports to a conversation once. The uniqueness is the whole point of
-- the table: the sweep that announces runs runs again after a crash, and
-- without a record of what left it would repeat every announcement it had
-- already made.
--
-- The row is written after the message leaves, never before. Slack's
-- chat.postMessage takes no idempotency key, so exactly-once is not available
-- at any price — and between the two failures that are, this picks the one
-- that is noise. A repeated approval request in a channel is an irritation; an
-- approval nobody ever saw is what the announcement exists to prevent.
--
-- Not in the ledger, deliberately. The ledger records what a *run* did; this
-- records what the platform managed to deliver about it, which is a fact about
-- a network and a bot token rather than about an agent's conduct. Putting it in
-- the chain would seal delivery receipts into the audit record and grow it by
-- one step per notification.
create table if not exists channel_deliveries (
    run_id       text        not null,
    event        text        not null,
    conversation text        not null,
    -- What the channel called the message. Stage 2 replies into this thread,
    -- so an approval lands under the thing it is about rather than as a loose
    -- message somewhere below it.
    ref          text        not null default '',
    posted_at    timestamptz not null default now(),

    primary key (run_id, event, conversation)
);

-- The sweep asks for runs that have not been reported, newest first.
create index if not exists channel_deliveries_posted_idx
    on channel_deliveries (posted_at desc);
