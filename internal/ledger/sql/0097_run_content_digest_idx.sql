-- fuseone:no-transaction
--
-- Finding what holds the same bytes as a run named in an erasure request.
--
-- A correction's copy and the rehearsals that replayed it hold the input of the
-- run they came from. The digest is the one link that survives the correction
-- being deleted, and the only one that does not mean reading the whole ledger.
-- Partial, because what has already been erased has nothing left to find.

create index concurrently if not exists run_content_digest_idx on run_content (digest)
    where erased_at is null;
