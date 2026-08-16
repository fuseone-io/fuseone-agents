-- One ask is opened once, however many consumers are reading.
--
-- Listing what is pending and acting on it is not a claim: two consumers read
-- the same row, both open a run and both reply, and the person who asked gets
-- answered twice by an agent that ran twice. `trigger.Opener` would catch the
-- duplicate run through its idempotency key — and the second reply, the second
-- refusal message and the second entry in the record are not runs and it
-- catches none of them.
--
-- The same shape the run queue already uses: a lease with an owner, and an
-- expired lease claimable again. A consumer that dies stops renewing and the
-- next sweep picks the ask up, so there is no reaper to write and none to
-- forget.
alter table channel_inbox
    add column if not exists leased_until timestamptz,
    add column if not exists lease_owner  text not null default '';

-- The consumer asks for what is pending and unleased, oldest first. Partial,
-- because settled rows are the ones that accumulate and none of them are ever
-- read this way again.
drop index if exists channel_inbox_pending_idx;

create index if not exists channel_inbox_claimable_idx
    on channel_inbox (at) where status = 'pending';
