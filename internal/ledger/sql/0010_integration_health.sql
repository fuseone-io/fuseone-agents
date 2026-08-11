-- What each connected system last said for itself.
--
-- Separate from the settings that configure a server: configuration is what an
-- operator wrote, and this is what the platform observed. Mixing them would
-- let an observation overwrite an intention, and the two answer different
-- questions — "what should this be" and "is it answering".
--
-- One row per server, overwritten on each observation. History of connections
-- is not kept here: a server that has flapped a thousand times is one row and
-- the administrative trail carries what people did about it.
create table if not exists integration_health (
    name        text        not null primary key,
    reachable   boolean     not null,
    tool_count  integer     not null default 0,
    detail      text        not null default '',
    observed_at timestamptz not null default now(),
    observed_by text        not null default ''
);
