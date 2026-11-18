-- fuseone:no-transaction
--
-- Indexes erasure reads to find the runs derived from a seal or from a
-- workbench thread (NT-018 phase 2, release 2a; columns from 0118).
--
-- Built concurrently, outside a transaction (the marker on the first line):
-- this runs while the previous release keeps appending runs, and a plain
-- create index would hold every write to runs for the length of a scan of
-- the whole table. Partial, because almost no run has either id.

create index concurrently if not exists runs_seal_idx on runs (seal_id)
    where seal_id is not null;

create index concurrently if not exists runs_authoring_thread_idx on runs (authoring_thread)
    where authoring_thread is not null;
