-- 0004_identity.sql
-- Scope, principals, grants, sessions, and the encrypted configuration the
-- administration area manages.
--
-- Everything an operator can change lives here rather than in environment
-- variables. A setting in the environment cannot be audited, cannot be scoped
-- to a company, and cannot be changed without a deploy — which is how a
-- platform ends up with its most consequential decisions in a Helm values file.

-- The scope hierarchy from PRD 3.1. Company exists from day one with a single
-- row; the second one is a configuration change, not a migration.
create table companies (
    company_id  text        not null primary key,
    name        text        not null,
    created_at  timestamptz not null default now(),
    archived_at timestamptz
);

create table areas (
    company_id  text        not null references companies(company_id),
    area_id     text        not null,
    name        text        not null,
    created_at  timestamptz not null default now(),
    archived_at timestamptz,
    primary key (company_id, area_id)
);

-- Principals: people, service accounts, and the agents that act on their
-- behalf. Subject is the identity provider's stable identifier — never the
-- email, which people change.
create table principals (
    principal_id text        not null primary key,
    kind         text        not null check (kind in ('user', 'service', 'agent')),
    provider     text        not null default '',
    subject      text        not null default '',
    display      text        not null default '',
    email        text,
    created_at   timestamptz not null default now(),
    last_seen_at timestamptz,
    disabled_at  timestamptz
);

-- One identity per provider. Two providers may legitimately use the same
-- subject string, so the pair is what must be unique.
create unique index principals_provider_subject_uniq
    on principals (provider, subject)
    where subject <> '';

-- Grants are always scoped. The installation-wide grant is a scope, not a
-- wildcard permission: role says what, scope says where, and the trail names
-- both for every administrative act.
create table role_grants (
    principal_id text        not null references principals(principal_id) on delete cascade,
    company_id   text        not null,
    area_id      text        not null,
    role         text        not null check (role in ('admin', 'author', 'approver', 'curator', 'auditor')),
    check (company_id <> '*' or area_id = ''),
    granted_by   text        not null default '',
    granted_at   timestamptz not null default now(),
    primary key (principal_id, company_id, area_id, role)
);

create index role_grants_scope_idx on role_grants (company_id, area_id);

-- Sessions are server-side. The browser holds an opaque identifier in an
-- httpOnly cookie and nothing else, so an XSS bug cannot exfiltrate a bearer
-- credential and revoking a session is a delete rather than a wait.
create table sessions (
    session_id   text        not null primary key,
    principal_id text        not null references principals(principal_id) on delete cascade,
    -- token_hash, never the token. A leaked database backup must not be a
    -- pile of usable session credentials.
    token_hash   bytea       not null,
    user_agent   text        not null default '',
    ip           text        not null default '',
    created_at   timestamptz not null default now(),
    last_used_at timestamptz not null default now(),
    expires_at   timestamptz not null,
    revoked_at   timestamptz
);

create index sessions_principal_idx on sessions (principal_id);
create index sessions_expiry_idx on sessions (expires_at) where revoked_at is null;

-- Long-lived credentials for CI, the CLI, and webhook senders. Same rule as
-- sessions: only the hash is stored.
create table api_tokens (
    token_id     text        not null primary key,
    principal_id text        not null references principals(principal_id) on delete cascade,
    name         text        not null,
    token_hash   bytea       not null,
    created_by   text        not null default '',
    created_at   timestamptz not null default now(),
    last_used_at timestamptz,
    expires_at   timestamptz,
    revoked_at   timestamptz
);

create unique index api_tokens_hash_uniq on api_tokens (token_hash);

-- Identity providers. Client secrets go in the encrypted settings table below,
-- never here.
create table identity_providers (
    provider_id  text        not null primary key,
    kind         text        not null check (kind in ('oidc', 'saml')),
    display      text        not null,
    issuer       text        not null default '',
    client_id    text        not null default '',
    metadata_url text        not null default '',
    -- Maps an assertion's group claim onto (company, area, role). Without it
    -- a successful login grants nothing, which is the correct default.
    group_mappings jsonb     not null default '[]'::jsonb,
    enabled      boolean     not null default false,
    created_at   timestamptz not null default now(),
    updated_at   timestamptz not null default now()
);

-- Everything the administration area configures: model providers, MCP
-- servers, capability packs, ceilings, retention.
--
-- Secrets are encrypted with AES-256-GCM under a key held outside the
-- database, so a stolen dump is not a stolen credential.
create table settings (
    scope_kind    text        not null check (scope_kind in ('installation', 'company', 'area')),
    company_id    text        not null default '',
    area_id       text        not null default '',
    kind          text        not null,
    name          text        not null,
    value         jsonb       not null default '{}'::jsonb,
    secret        bytea,
    secret_nonce  bytea,
    enabled       boolean     not null default true,
    updated_by    text        not null default '',
    updated_at    timestamptz not null default now(),
    primary key (scope_kind, company_id, area_id, kind, name)
);

create index settings_kind_idx on settings (kind, enabled);

-- Every administrative change is recorded. The run ledger covers what agents
-- did; this covers what operators did to the platform, and an auditor needs
-- both to explain any outcome.
create table admin_events (
    event_id     bigserial   primary key,
    at           timestamptz not null default now(),
    principal_id text        not null,
    company_id   text        not null default '',
    area_id      text        not null default '',
    action       text        not null,
    target       text        not null default '',
    detail       jsonb       not null default '{}'::jsonb
);

create index admin_events_at_idx on admin_events (at desc);
create index admin_events_target_idx on admin_events (target, at desc);
