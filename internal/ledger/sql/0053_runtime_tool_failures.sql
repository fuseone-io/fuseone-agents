-- /runtime needs to distinguish MCP/tool failures while an incident is active.
-- Folding run_steps on that page would make the database do archival work at
-- the moment somebody is trying to understand an outage, so keep the only
-- trail shape that page reads indexed by the way it reads it: newest first,
-- failed tool returns only.

create index if not exists run_steps_tool_returned_failed_at_idx
    on run_steps (at desc, run_id desc, seq desc)
    where kind = 'tool_returned'
      and payload->>'failed' = 'true';
