-- Why an ask became nothing, as a code rather than a sentence.
--
-- The sentence is what the person is told and it is written for them: it names
-- the agent they meant, or the limit they reached, or what would have worked.
-- None of that groups. An operator asking "why is this conversation quiet"
-- needs to count, and counting sentences with names in them counts nothing.
--
-- It also decides who gets told. A limit that answers every message it rejects
-- amplifies the flood it exists to stop, so the second refusal of the same kind
-- inside the same window is recorded and not said — which is a question about
-- what kind, and the sentence cannot answer it.
alter table channel_inbox
    add column if not exists reason text not null default '';

-- Counting one correspondent's recent runs, which is the ceiling's whole
-- question. Partial, because the pending and refused rows are not what it
-- counts and the table is mostly those.
create index if not exists channel_inbox_opened_by_idx
    on channel_inbox (channel, asked_by, at) where status = 'opened';

-- And whether they have already been told. Same shape, other half.
create index if not exists channel_inbox_told_idx
    on channel_inbox (channel, asked_by, reason, at) where status = 'refused';
