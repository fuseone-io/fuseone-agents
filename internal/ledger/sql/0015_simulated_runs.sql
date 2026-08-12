-- Simulated runs, marked and excluded by default.
--
-- FU-10 refuses to let an agent leave Draft without a reviewed simulation, so
-- a simulation has to persist and be readable — which means it belongs in the
-- ledger somebody already reads, with the trail and the diagram working on it
-- unchanged. Building a second ledger for it would duplicate the best part of
-- the product and then have to be kept in step with it.
--
-- The risk that buys is contamination: a simulated run counted as a real one
-- in a cost report is a wrong number somebody acts on. So the column is NOT
-- NULL with a default, every listing filters on it, and the partial index
-- below is the one the run list actually uses — a query that forgets the
-- filter loses the index and gets slow, which is a nudge in the right
-- direction rather than a silent wrong answer.
alter table runs add column if not exists simulated boolean not null default false;

create index if not exists runs_real_started_idx
    on runs (started_at desc) where not simulated;
