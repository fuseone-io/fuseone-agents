-- 0045_run_failure_summary.sql
-- The projection keeps the stable part of the latest failed turn beside the
-- run so lists and runtime health can explain an outage without folding every
-- step.
-- Raw provider text stays in last_error; these columns are low-cardinality
-- fields a screen and dashboard can safely group.

alter table runs
    add column if not exists failure_code text,
    add column if not exists failure_provider text,
    add column if not exists failure_status int,
    add column if not exists failure_request_id text,
    add column if not exists failure_retryable boolean;

create index if not exists runs_failure_updated_idx
    on runs (failure_code, updated_at desc)
    where failure_code is not null;
