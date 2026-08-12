-- Which simulation a run belonged to.
--
-- The report is a fold of the ledger, so nothing about a simulation is stored
-- twice — but the runs of one batch have to be findable again, and a run
-- cannot say on its own which of an agent's simulations opened it. The mark
-- in 0015 says a run touched nothing; this says which batch it answered for.
--
-- Empty for every real run, and every query for a batch is a query for a
-- non-empty id: asking for '' must never return production.
alter table runs add column if not exists simulation text not null default '';

create index if not exists runs_simulation_idx
    on runs (simulation, started_at, run_id) where simulation <> '';
