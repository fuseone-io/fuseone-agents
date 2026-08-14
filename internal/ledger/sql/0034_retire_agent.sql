-- Taking an agent out of circulation.
--
-- Not deleting it, and the difference is the whole point: every run is pinned
-- to a version, and that version is the only correct explanation of what the
-- run did. Removing the definition would leave the ledger pointing at a text
-- nobody can read — the opposite of what this platform is for.
--
-- So a retired agent disappears from the lists, cannot be started, and stays
-- entirely readable: its versions, its runs, and the trail entry saying who
-- retired it and when.

alter table agent_state add column if not exists retired_at timestamptz;
alter table agent_state add column if not exists retired_by text not null default '';
