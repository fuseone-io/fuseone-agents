-- Discovery and tools/call answer different questions. Discovery uses the
-- connection credential and imports the tool list; a concrete call may still
-- fail later because the run principal lacks a personal credential, a rate
-- limit fired, or the server failed under load.
alter table integration_health
    add column if not exists last_reachable_at timestamptz,
    add column if not exists tool_call_ok boolean,
    add column if not exists tool_call_code text not null default '',
    add column if not exists tool_call_observed_at timestamptz,
    add column if not exists tool_call_observed_by text not null default '',
    add column if not exists last_tool_call_ok_at timestamptz;

update integration_health
set last_reachable_at = observed_at
where reachable and last_reachable_at is null;
