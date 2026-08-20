-- Successful asks from channels owe their closing answer back to the thread
-- that asked. Existing opened rows predate that contract; marking the debt
-- explicitly keeps an upgrade from replaying old run outcomes into channels.
alter table channel_inbox
    add column if not exists answer_due boolean not null default false;

create index if not exists channel_inbox_finished_answer_idx
    on channel_inbox (at)
    where status = 'opened' and answer_due and answered_at is null;
