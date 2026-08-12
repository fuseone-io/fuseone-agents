-- Whether an agent runs, which is not part of what it is.
--
-- A specification is immutable and versioned: it is what somebody wrote, and
-- every run is pinned to one. Whether that agent is allowed to start is
-- operational and changes on a Tuesday afternoon because something is on fire.
-- Versioning it would make pausing an agent an act of authorship, and would
-- put a new version between a run and the text it actually ran.
--
-- So it lives here, one row per agent, no history of its own. What people did
-- about it is in the administrative trail, where decisions belong.
create table if not exists agent_state (
    agent_id   text        not null primary key,
    paused     boolean     not null default true,
    changed_at timestamptz not null default now(),
    changed_by text        not null default ''
);
