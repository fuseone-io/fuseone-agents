-- The projection carried a total and a token count, which is enough to bill a
-- run and not enough to explain one. A cache read costs a fraction of an input
-- token, so without the split an expensive agent cannot be diagnosed — and the
-- cost API promises the split.
--
-- Existing rows keep zeroes in the new columns: the breakdown was never
-- recorded for them, and back-filling from the ledger would be a migration
-- that walks every step of every run. The totals they already carry stay
-- correct, so nothing that was true stops being true.
alter table runs
    add column if not exists input_tokens       bigint not null default 0,
    add column if not exists output_tokens      bigint not null default 0,
    add column if not exists cache_read_tokens  bigint not null default 0,
    add column if not exists cache_write_tokens bigint not null default 0;

-- Listing runs filters on scope and orders by recency; without this it is a
-- sequential scan of the whole history on every page view.
create index if not exists runs_recent_idx on runs (started_at desc);
create index if not exists runs_scope_idx on runs (company_id, area_id, started_at desc);
create index if not exists runs_agent_idx on runs (agent_id, started_at desc);
