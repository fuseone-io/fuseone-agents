-- The stages a definition declares.
--
-- Parsed, validated and rendered since the beginning, and never stored: the
-- table had no column, so publishing an agent with steps kept them exactly as
-- long as the process that parsed them. Reading one back gave a specification
-- with none, which is a different agent from the one somebody wrote.
--
-- What they are for is narrower than it looks and it is the reason this
-- matters: `reaches` is what the Gate is meant to allow while a run sits at
-- that step, so the capability pack is the ceiling and the step is the actual
-- permission (NT-003 §8). Storing them is the first half of that; obeying
-- them is the second and is not in this migration.

alter table agent_specs add column if not exists steps jsonb not null default '[]'::jsonb;
