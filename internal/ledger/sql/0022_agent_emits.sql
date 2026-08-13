-- Events a finished run of an agent publishes (PRD SE-10).
--
-- A column rather than re-parsing the stored source on every read: the
-- composition graph asks this of every published agent at once, and the
-- triggers beside it are stored the same way for the same reason.
--
-- Existing rows get the empty array, which is the truth about them: they were
-- published before an agent could declare an event, so they declare none.
alter table agent_specs
    add column if not exists emits text[] not null default '{}';
