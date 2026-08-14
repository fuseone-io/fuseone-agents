-- The last battery run against a version.
--
-- Asked every time somebody starts an agent, and answered from the runs
-- rather than from a table beside them: a simulation is exactly the set of
-- runs that name it, and a record kept alongside could disagree with them.
--
-- Partial, on `simulated`. Simulated runs are a small minority of a table
-- that grows with everything the installation ever did, so an index over all
-- of them would be mostly pages this query never reads.

create index if not exists runs_last_battery_idx
    on runs (agent_id, version_id, started_at desc)
    where simulated and simulation <> '';
