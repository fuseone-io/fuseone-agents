-- The pair a draft or copilot run switches to when the declared provider keeps
-- failing for reasons a retry is meant to outlast (#27). Frozen with the
-- version like every other column here. Empty declares none, which is how
-- every version published before this behaves.
alter table agent_specs
    add column fallback_provider text not null default '',
    add column fallback_model    text not null default '';
