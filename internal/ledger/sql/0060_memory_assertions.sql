create table if not exists memory_assertion_events (
	id bigserial primary key,
	assertion_id text not null,
	action text not null check (action in ('asserted', 'disabled', 'source_erased')),
	company_id text not null,
	area_id text not null,
	agent_id text not null default '',
	principal_id text not null default '',
	reason text not null default '',
	detail jsonb not null default '{}'::jsonb,
	at timestamptz not null default now()
);

create index if not exists memory_assertion_events_assertion_idx
    on memory_assertion_events (assertion_id, id);
create index if not exists memory_assertion_events_at_idx
    on memory_assertion_events (at);

create table if not exists memory_assertions (
	assertion_id text primary key,
	company_id text not null,
	area_id text not null,
	agent_id text not null default '',
	kind text not null,
	subject text not null,
	signature text not null,
	claim text not null,
	evidence jsonb not null default '[]'::jsonb,
	observations bigint not null default 0,
	confirmed bigint not null default 0,
	labels text[] not null default '{}',
	status text not null check (status in ('active', 'disabled', 'expired', 'source_erased')),
	expires_at timestamptz,
	created_by text not null default '',
	created_at timestamptz not null,
	updated_by text not null default '',
	updated_at timestamptz not null
);

create unique index if not exists memory_assertions_identity_idx
	on memory_assertions (company_id, area_id, agent_id, kind, subject, signature);

create index if not exists memory_assertions_lookup_idx
	on memory_assertions (company_id, area_id, agent_id, kind, subject, signature)
	where status = 'active';

create index if not exists memory_assertions_expires_idx
    on memory_assertions (expires_at)
    where expires_at is not null and status = 'active';
create index if not exists memory_assertions_updated_idx
    on memory_assertions (updated_at);
