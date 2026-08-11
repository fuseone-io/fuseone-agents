-- The claim check: payloads too bulky or too sensitive for the ledger.
--
-- They lived in a map inside the worker process, which meant a run that had
-- already called a tool could not be resumed by any other process — including
-- the same worker after a restart. Rebuilding the transcript needs the earlier
-- tool arguments and results, so resuming failed outright. That contradicts
-- NF-02 (an abrupt crash at any point resumes) and DE-15 (an upgrade does not
-- interrupt runs in flight).
--
-- The ledger still records only a reference and a digest (AU-04); this is
-- where the bytes live, under the installation's own retention.
create table run_content (
    ref        text        not null primary key,
    run_id     text        not null,
    seq        bigint      not null,
    digest     text        not null,
    bytes      bytea       not null,
    created_at timestamptz not null default now()
);

-- Retention and per-subject erasure both work per run (AU-11, NF-09), which is
-- why the reference is run-scoped rather than a bare content hash: two runs
-- holding identical bytes must be purgeable independently.
create index run_content_run_idx on run_content (run_id);
