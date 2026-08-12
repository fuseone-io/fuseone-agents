-- Content that belongs to something other than a run.
--
-- Simulation cases are real customer records — an email, a ticket — and the
-- claim check is exactly their shape: bulky, sensitive, held outside the
-- ledger as a reference and a digest under the installation's retention
-- (AU-04). A table of their own would be a second place for personal data to
-- accumulate, with its own retention nobody remembers to set and its own
-- erasure path nobody remembers to run.
--
-- So the owner becomes a pair rather than a run. The existing column keeps its
-- name because renaming it would touch every query for no behaviour: read
-- `run_id` as the owner's id, and `owner_kind` as what kind of thing it is.
alter table run_content add column if not exists owner_kind text not null default 'run';

create index if not exists run_content_owner_idx on run_content (owner_kind, run_id);
