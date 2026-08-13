-- Which version an agent runs, which is not the newest one ever published.
--
-- "Latest" was derived from published_at, and that is wrong twice. Rolling
-- back is selecting an earlier version (PRD DE-08), and a derived latest
-- cannot express a selection. Worse, a version that was published and then
-- withdrawn — an author edited a file and reverted it — stays the newest by
-- timestamp for ever: every new run pins to a specification nobody holds any
-- more, and parks with spec_unresolved seconds after it opens.
--
-- So it lives beside the spec, like paused and stage, for the same reason
-- those do: a published version is what somebody wrote and never changes,
-- while which one is current changes on an afternoon.
--
-- Null means nobody has chosen, which is the state of an installation
-- upgrading into this: the newest by published_at is still the answer, and
-- the first publication after the upgrade names one.
alter table agent_state
    add column if not exists current_version text;
