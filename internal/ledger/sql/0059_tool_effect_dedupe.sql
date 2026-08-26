create table if not exists tool_effect_dedupe (
	company_id text not null,
	area_id text not null,
	agent_id text not null,
	tool_id text not null,
	fingerprint text not null,
	status text not null check (status in ('pending', 'confirmed')),
	run_id text not null,
	seq bigint not null default 0,
	reserved_at timestamptz not null,
	confirmed_at timestamptz,
	expires_at timestamptz not null,
	updated_at timestamptz not null,
	primary key (company_id, area_id, agent_id, tool_id, fingerprint)
);

create index if not exists tool_effect_dedupe_expires_idx
	on tool_effect_dedupe (expires_at);

create index if not exists tool_effect_dedupe_confirmed_run_idx
	on tool_effect_dedupe (run_id, seq)
	where status = 'confirmed';
