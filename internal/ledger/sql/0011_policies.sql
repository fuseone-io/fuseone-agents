-- Authored policies, and the snapshots that make a decision reconstructable.
--
-- Two tables because they answer different questions. `policies` is what is in
-- force now and what an operator edits. `policy_snapshots` is what was in force
-- at a moment, keyed by the hash every ledger step already carries — which is
-- what turns `policy_hash` from a label into something an auditor can fetch and
-- re-evaluate against (PRD AU-08).
--
-- Snapshots are never deleted. A run from two years ago references one, and the
-- promise is that its decision can be reconstructed under the policy of its
-- time — not under whatever survived a cleanup.
create table if not exists policies (
    code        text        not null primary key,
    company_id  text        not null default '',
    area_id     text        not null default '',
    name        text        not null,
    owner       text        not null default '',
    reason      text        not null default '',
    resource    text        not null default '*',
    effects     text[]      not null default '{}',
    reach       text        not null default 'installation',
    scopes      jsonb       not null default '[]'::jsonb,
    agents      text[]      not null default '{}',
    conditions  jsonb       not null default '[]'::jsonb,
    effect      text        not null,
    mode        text        not null default 'monitor',
    enabled     boolean     not null default true,
    created_at  timestamptz not null default now(),
    updated_at  timestamptz not null default now(),
    updated_by  text        not null default ''
);

create table if not exists policy_snapshots (
    policy_hash text        not null primary key,
    taken_at    timestamptz not null default now(),
    policies    jsonb       not null
);

create index if not exists policy_snapshots_taken_idx
    on policy_snapshots (taken_at desc);
