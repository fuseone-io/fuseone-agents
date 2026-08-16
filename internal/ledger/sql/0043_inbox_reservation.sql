-- The slot an ask holds against its correspondent's ceiling.
--
-- Counting and then opening is a check followed by a decision, and between them
-- is every other worker. Two of them see that nobody has spent anything, both
-- open, and a ceiling of one lets through as many runs as there are processes
-- sweeping. It goes wrong exactly when it matters: a flood is what puts several
-- asks in the inbox at once, which is what gives two workers something to claim.
--
-- So the slot is taken before the run is opened, under a lock held per
-- correspondent, and the taking is what the next decision counts.
--
-- It expires with the lease and not with the window. A worker that dies holding
-- a reservation would otherwise keep it for an hour, and twenty crashes would
-- be an hour of silence for somebody who did nothing wrong. The lease already
-- says whether anybody is still working on this ask; a second answer to the
-- same question is a second thing to get wrong.
alter table channel_inbox
    add column if not exists reserved_at timestamptz;

-- What the count reads: the slots this correspondent is holding right now.
create index if not exists channel_inbox_reserved_idx
    on channel_inbox (channel, asked_by)
    where status = 'pending' and reserved_at is not null;
