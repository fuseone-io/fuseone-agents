-- Promotion evidence leaves out runs that switched to their fallback pair
-- (#27), and asks it with a probe for a model_fallback step on every run it
-- weighs: agreement, version agreement, gate blocks and the Trust Center's
-- activity. No index covers that kind, so the probe planned as an anti-join
-- over a sequential scan of every run_steps partition — several times per
-- Trust Center page and again on the promotion sweep, growing with the ledger.
--
-- Partial, because the kind is rare: the index holds one row per run that fell
-- back and nothing else, and the probe reads it as an index-only scan.
--
-- Created on the parent like the trail indexes (0025, 0053), which cascades to
-- every partition and to the ones created later. Not concurrently: migrations
-- run in a transaction, and a partitioned parent refuses it anyway.
create index if not exists run_steps_model_fallback_run_idx
    on run_steps (run_id)
    where kind = 'model_fallback';
