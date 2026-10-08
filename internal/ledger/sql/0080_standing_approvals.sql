-- Standing approvals: a human grant given ahead of time (PRD SE-06's one
-- exception, made durable). Each row is one person pre-approving one exact
-- tool for one agent in one scope, with a daily ceiling and an expiry.
-- Revocation flips status and keeps the row: the trail must show both the
-- mandate and its end.

create table if not exists standing_approvals (
	grant_id text primary key,
	tool text not null,
	agent_id text not null,
	company_id text not null,
	area_id text not null default '',
	daily_cap int not null check (daily_cap between 1 and 100),
	reason text not null check (reason <> ''),
	status text not null check (status in ('active', 'revoked')),
	created_by text not null,
	created_at timestamptz not null,
	expires_at timestamptz not null,
	revoked_by text not null default '',
	revoked_at timestamptz
);

create index if not exists standing_approvals_cover_idx
	on standing_approvals (tool, agent_id, status, expires_at);

create index if not exists standing_approvals_scope_idx
	on standing_approvals (company_id, area_id, status);
