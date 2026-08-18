-- 0045_run_failure_summary.sql
-- The projection keeps the stable part of the latest failed turn beside the
-- run so lists and runtime health can explain an outage without folding every
-- step.
-- Classified provider failures also write the stable code to last_error,
-- leaving provider bodies out of the run projection.

alter table runs
    add column if not exists failure_code text,
    add column if not exists failure_provider text,
    add column if not exists failure_status int,
    add column if not exists failure_request_id text,
    add column if not exists failure_retryable boolean;

create index if not exists runs_failure_updated_idx
    on runs (failure_code, updated_at desc)
    where failure_code is not null;

create index if not exists runs_runtime_active_phase_idx
    on runs (phase, updated_at desc)
    where not simulated
      and phase in ('running', 'awaiting_tool', 'awaiting_approval', 'parked', 'compensating');

create index if not exists runs_runtime_terminal_updated_idx
    on runs (updated_at desc, phase)
    where not simulated
      and phase in ('finished', 'failed');

create index if not exists runs_runtime_claimable_idx
    on runs (next_attempt_at, leased_until)
    where not simulated
      and phase in ('running', 'awaiting_tool', 'compensating');
