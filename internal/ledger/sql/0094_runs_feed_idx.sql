-- The event feed's index holds only what the feed can ever offer (#228).
--
-- 0092 indexed every run with a finished_xid. Simulations get one too — the
-- trigger stamps whoever reaches 'finished' — but the feed never offers them,
-- so the cursor never moves past them. Behind fifty thousand simulations the
-- next real run was fifty thousand index entries away, and every feed page
-- and every backlog reading walked all of them, filtering after the walk, for
-- as long as the simulations stayed there: for ever.
--
-- The predicate is the feed's own, word for word — finished_xid is not null,
-- phase = 'finished', not simulated — so the planner can prove both the
-- feed page (FinishedSince) and the backlog reading are covered by it, and
-- the limit counts real finished runs and nothing else.
--
-- Not concurrently: migrations run in one transaction with their record in
-- schema_migrations, and create index concurrently refuses a transaction.
-- The build scans runs once under a share lock that holds writes to runs
-- for its duration; the index itself holds only real finished runs since
-- 0092, so it is small.
--
-- The 0092 index goes: nothing reads finished_xid but these two statements,
-- and a binary rolled back to 0.86 sends the same predicate, which this index
-- covers. Keeping both would only double the writes on every finish.
--
-- Numbered 0094 because #231 takes 0093. The runner applies whatever is
-- unapplied in filename order, so either may reach an installation first.
create index runs_feed_idx on runs (finished_xid, run_id)
    where finished_xid is not null and phase = 'finished' and not simulated;

drop index runs_finished_xid_idx;
