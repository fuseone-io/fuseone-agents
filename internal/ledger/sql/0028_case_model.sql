-- What the case last held against.
--
-- The corpus can say a version stopped doing the right thing. It cannot say
-- why, and there are two very different reasons: somebody changed the agent,
-- or a provider changed the model under a name that did not change. The second
-- is the one nobody notices, because nothing in this installation moved.
--
-- So a case records the model it passed against. A battery whose broken count
-- rises while every case still names the model it always did is a change
-- somebody made; one where the model moved underneath is drift, and the
-- difference decides who gets woken up.
--
-- Nullable and empty for the cases already recorded: they held against
-- something, and inventing which model would be worse than admitting we do not
-- know.
alter table regression_cases
    add column if not exists model  text not null default '',
    add column if not exists effort text not null default '';
