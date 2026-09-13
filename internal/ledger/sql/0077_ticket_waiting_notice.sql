-- A saved revision cannot open while an earlier revision still owns the
-- ticket. Attempts and the separately acknowledged notice keep that event
-- pending while telling its requester once that it is waiting; the notice is
-- not a terminal disposition of the event.
alter table channel_inbox
    add column if not exists claim_attempts integer not null default 0
        constraint channel_inbox_claim_attempts_nonnegative check (claim_attempts >= 0),
    add column if not exists pending_notice text not null default '',
    add column if not exists pending_notice_answered_at timestamptz;

-- 0076 gave the second result shape a stable name. Version it now so a future
-- replacement can name exactly which historical shape it supersedes without
-- depending on a PostgreSQL-generated constraint name.
alter table governed_external_attempts
    rename constraint governed_external_attempts_result_shape
    to governed_external_attempts_result_shape_v2;
