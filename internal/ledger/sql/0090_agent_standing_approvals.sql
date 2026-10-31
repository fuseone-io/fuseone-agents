-- Whether a standing approval may ever release a call of this version (#192).
-- Frozen with the version like every other column here: the claim reads the
-- version the run pinned, and creating a grant reads the agent's current one.
-- Empty allows grants, which is how every version published before this
-- behaves; 'forbidden' is the only other value a definition can declare.
alter table agent_specs
    add column standing_approvals text not null default ''
        check (standing_approvals in ('', 'forbidden'));
