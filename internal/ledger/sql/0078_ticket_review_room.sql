-- A ticket may be worked in a room of its own before its answer is published.
--
-- The room is the conversation's configuration at the moment the ticket opened,
-- kept on the ticket for the reason the addressing source is: a rule edited
-- later must not move a thread that is already open. The thread inside the room
-- is opened once, by whichever worker claims the obligation, and the lease makes
-- worker death retryable. Marking before posting would make a lost process
-- indistinguishable from an opened thread.
alter table governed_tickets
    add column review_conversation text not null default '',
    add column review_root text not null default '',
    add column review_claimed_by text not null default '',
    add column review_claim_until timestamptz,
    add constraint governed_tickets_review_root_needs_room
        check (review_root = '' or review_conversation <> '');

-- Replies in the room find their ticket the same way replies in the support
-- thread do: one indexed equality lookup, never a scan.
create unique index governed_tickets_review_idx
    on governed_tickets (origin_connection, review_conversation, review_root)
    where review_conversation <> '' and review_root <> '';

-- The sweep reads exactly the rooms that still owe a thread.
create index governed_tickets_review_pending_idx
    on governed_tickets (updated_at)
    where review_conversation <> '' and review_root = '';
