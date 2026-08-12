-- Erasure reaches the content and never the step that references it.
--
-- That is what answers the open question the PRD raised about NF-09: erasing
-- personal data cannot invalidate the hash chain, because personal data was
-- never in the chain. The step keeps a reference and a digest (AU-04); this
-- table keeps the bytes, and only the bytes go.
--
-- A tombstone rather than a deleted row. Erased and never-stored are different
-- facts: one is a deletion somebody performed, under retention or on a
-- subject's request, and the other is a reference that was always wrong. An
-- auditor reading a trail that points at nothing has to be able to tell them
-- apart, and the digest left behind still proves what the bytes were to
-- anybody holding a copy.
alter table run_content alter column bytes drop not null;
alter table run_content add column if not exists erased_at timestamptz;
alter table run_content add column if not exists erased_reason text not null default '';

-- Retention sweeps by age across every owner, so the scan is by creation time.
create index if not exists run_content_created_idx
    on run_content (created_at) where erased_at is null;
