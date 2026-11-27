-- 0129_integration_health_awaiting_person.sql
-- A server only a person can discover, observed with nobody connected to
-- discover as, waits for a person (#483, delivery 3b). The observation keeps
-- the server configuration's updated_at it was made under: a reader honours it
-- only while that is still the configuration, so an observation the previous
-- binary leaves behind — it never writes these columns — goes stale the moment
-- the server changes, and the reader also requires the row to say unreachable.
--
-- Additive only: the previous binary's insert and upsert name their columns,
-- and a defaulted column they never mention is no change to them.

alter table integration_health
    add column if not exists awaiting_person boolean not null default false,
    add column if not exists awaiting_generation timestamptz;
