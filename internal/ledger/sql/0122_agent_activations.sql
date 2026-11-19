-- Every activation of a version, and which seal it read (NT-018 phase 2, release 2c).
--
-- Activation makes a version current through the publication's own locks and checks.
-- On the candidate path it reads a seal's bytes and writes them into agent_specs; on
-- the published path it makes an existing row current again and reads no seal. This is
-- the provenance of either: one row per activation that committed, written in its
-- transaction.
--
-- Insert-only, holding ids and references only. No text a person wrote lives here, so
-- an erasure has nothing to reach: the seal's own bytes are erased through the seal, and
-- the published row is out of erasure's reach as every published version is.
--
-- A plain table in the migration's transaction: nothing reads or writes it before this
-- release.
create table agent_activations (
    activation_id uuid        not null primary key default gen_random_uuid(),
    agent_id      text        not null,
    version_id    text        not null,
    -- The seal read on the candidate path; null on the published path.
    seal_id       uuid        references agent_seals (seal_id),
    path          text        not null check (path in ('candidate', 'published')),
    activated_by  text        not null,
    activated_at  timestamptz not null default now(),

    foreign key (agent_id, version_id) references agent_specs (agent_id, version_id),
    check ((path = 'candidate') = (seal_id is not null))
);
create index agent_activations_agent_idx on agent_activations (agent_id, activated_at);

create trigger agent_activations_permanent
    before update or delete on agent_activations
    for each row execute function agent_seals_are_permanent();
