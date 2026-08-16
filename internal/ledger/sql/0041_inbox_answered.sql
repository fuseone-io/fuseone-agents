-- A refusal that has been recorded and not yet said.
--
-- Two rules pulled against each other and the ordering could satisfy only one
-- at a time. Record first and a driver failure leaves the ask closed with
-- nobody told. Answer first and the reply goes out before ownership is proven,
-- so a worker whose lease lapsed posts a refusal the worker that took over
-- posts again — the duplicated attention the claim exists to prevent, moved
-- one step earlier.
--
-- Neither ordering fixes it, because the problem is that two things were being
-- done at once. So the refusal is recorded by whoever holds the ask, and the
-- saying of it becomes work of its own: owed until delivered, claimed the same
-- way the ask was, retried by whoever picks it up.
alter table channel_inbox
    add column if not exists answered_at timestamptz;

-- What has been refused and not yet said. Partial and small: it holds only
-- what is owed, and the ordinary state of this column is answered.
create index if not exists channel_inbox_owed_idx
    on channel_inbox (at) where status = 'refused' and answered_at is null;
