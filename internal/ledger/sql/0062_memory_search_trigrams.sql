-- Runtime memory lookup keeps substring semantics, so a plain btree cannot
-- help the search field. Trigram indexes support ILIKE without turning the
-- remembered claim into a different full-text language.
create extension if not exists pg_trgm;

create index if not exists memory_assertions_subject_trgm_idx
    on memory_assertions using gin (subject gin_trgm_ops)
    where status = 'active';

create index if not exists memory_assertions_signature_trgm_idx
    on memory_assertions using gin (signature gin_trgm_ops)
    where status = 'active';

create index if not exists memory_assertions_claim_trgm_idx
    on memory_assertions using gin (claim gin_trgm_ops)
    where status = 'active';
