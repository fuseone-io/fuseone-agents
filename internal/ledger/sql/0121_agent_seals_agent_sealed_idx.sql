-- The candidate listing's order (NT-018 phase 2, release 2a): an agent's seals,
-- newest first, read under a limit.
--
-- agent_seals is insert-only, so without this a listing would sort the agent's
-- whole sealing history to answer its first page. A plain index inside the
-- migration's transaction, as 0120: nothing writes agent_seals before release
-- 2b, so the table is empty when this runs.
create index agent_seals_agent_sealed_idx on agent_seals (agent_id, sealed_at desc, seal_id desc);
