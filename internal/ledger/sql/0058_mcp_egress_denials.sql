create table if not exists mcp_egress_denials (
	server text not null,
	host text not null,
	port integer not null,
	code text not null,
	attempts bigint not null default 1,
	first_seen timestamptz not null default now(),
	last_seen timestamptz not null default now(),
	primary key (server, host, port, code)
);

create index if not exists mcp_egress_denials_last_seen_idx
	on mcp_egress_denials (last_seen desc, code);
