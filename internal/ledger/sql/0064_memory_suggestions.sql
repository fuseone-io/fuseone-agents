alter table memory_assertion_events
	drop constraint if exists memory_assertion_events_action_check;

alter table memory_assertion_events
	add constraint memory_assertion_events_action_check check (
		action in (
			'asserted',
			'disabled',
			'source_erased',
			'suggested',
			'accepted',
			'dismissed',
			'auto_confirmed'
		)
	);

create table if not exists memory_suggestions (
	suggestion_id text primary key,
	assertion_id text not null,
	company_id text not null,
	area_id text not null,
	agent_id text not null default '',
	kind text not null,
	subject text not null,
	signature text not null,
	claim text not null,
	evidence jsonb not null default '[]'::jsonb,
	observations bigint not null default 0,
	labels text[] not null default '{}',
	status text not null check (status in (
		'pending',
		'accepted',
		'dismissed',
		'auto_confirmed',
		'source_erased'
	)),
	expires_at timestamptz,
	created_by text not null default '',
	created_at timestamptz not null,
	updated_by text not null default '',
	updated_at timestamptz not null
);

create index if not exists memory_suggestions_scope_status_idx
	on memory_suggestions (company_id, area_id, status, updated_at desc, suggestion_id);

create index if not exists memory_suggestions_assertion_idx
	on memory_suggestions (assertion_id, status);

create index if not exists memory_suggestions_updated_idx
	on memory_suggestions (updated_at);

create index if not exists memory_suggestions_expires_idx
	on memory_suggestions (expires_at)
	where expires_at is not null;
