-- Asks that arrived from a conversation, written down before they are
-- acknowledged.
--
-- Slack wants a 2xx within three seconds and retries what it does not get,
-- which is why a run cannot be opened on that request. But moving the work off
-- the request only solves the retry: between acknowledging and opening there
-- is a window, and a process that dies inside it has told the sender the ask
-- arrived and holds no record that it did. The sender is satisfied and the
-- question is gone — a failure that reports success, which is the worst pair
-- available.
--
-- So the order is: verify the signature, write the row, commit, acknowledge.
-- Opening happens afterwards and may be a different process or a later one.
--
-- Not in the ledger. The ledger records what a run did, and most of these
-- never become a run: a mention naming no agent, an ask in a conversation
-- nobody mapped, a retry of something already handled. Sealing those into the
-- chain would fill an audit record with other people's typing.
create table if not exists channel_inbox (
    channel   text not null,
    -- The conversation and the sender's own identifier for this delivery. The
    -- pair is what makes a retry cost nothing: Slack redelivers, the insert
    -- conflicts, and the same acknowledgement goes back.
    --
    -- The vendor's id and not one of ours, because the sender is the only
    -- party who knows that two deliveries are the same delivery.
    conversation text not null,
    event_id     text not null,

    -- What arrived, and the digest of it. The digest is what a later reader
    -- checks the payload against; the payload is what an ask is built from and
    -- is subject to retention like any other content the platform holds.
    payload bytea not null,
    digest  text  not null,

    -- pending, opened, or refused. A refusal is kept rather than deleted:
    -- "somebody mentioned an agent that cannot be started here" is a fact an
    -- operator will want when the person complains nothing happened.
    status text        not null default 'pending',
    -- Why, for a refusal. Empty otherwise.
    detail text        not null default '',
    run_id text        not null default '',
    at     timestamptz not null default now(),

    primary key (channel, conversation, event_id)
);

-- The consumer asks for what has not been opened, oldest first: an ask that
-- arrived before another was asked first, and answering them out of order in a
-- conversation reads as the platform ignoring somebody.
create index if not exists channel_inbox_pending_idx
    on channel_inbox (at) where status = 'pending';
