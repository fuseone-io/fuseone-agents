-- The installation-wide /runtime view reads failed tool returns newest first,
-- and 0053 indexes that shape. Narrow callers add company/area filters; without
-- this partial index they still walk the installation's incident window and
-- discard what they cannot read.

create index if not exists run_steps_tool_returned_failed_scope_at_idx
    on run_steps (company_id, area_id, at desc, run_id desc, seq desc)
    where kind = 'tool_returned'
      and payload->>'failed' = 'true';
