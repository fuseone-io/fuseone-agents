-- How an agent's owner asked for human approval to arrive.
--
-- Stored with the version rather than beside the agent, because it is part of
-- what was published: a run is governed by the version it pinned, and reading
-- today's preference would answer a question about a run with a decision taken
-- after it started.
--
-- The default is the console alone, which is how every agent behaved before one
-- could ask for anything else.
alter table agent_specs
	add column if not exists approvals jsonb not null
	default '{}'::jsonb;
