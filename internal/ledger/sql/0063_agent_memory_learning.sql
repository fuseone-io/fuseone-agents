alter table agent_specs
	add column if not exists memory_learning jsonb not null
	default '{"mode":"off"}'::jsonb;
