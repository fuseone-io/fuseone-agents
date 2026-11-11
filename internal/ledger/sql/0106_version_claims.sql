-- Which bytes a version id names (NT-018, release C1).
--
-- A version id is 48 bits of the SHA-256 of a definition's bytes, and
-- agent_specs is insert-or-nothing on that id. A second definition landing on
-- the same id would be answered with the first one's row: a publication would
-- judge one text and make the other current. A claim records the full digest
-- once, and every writer compares against it before it judges anything.
--
-- The primary key is what makes a claim atomic: two writers with different
-- digests cannot both claim one pair.
create table agent_version_digests (
    agent_id   text        not null,
    version_id text        not null,
    -- NULL for a version published before claims existed: its bytes were never
    -- kept, and serialising the row would not recover them. Nothing can match a
    -- NULL claim, so such an id is never published or made current again
    -- through a claim.
    digest     text        check (digest ~ '^[0-9a-f]{64}$'),
    claimed_at timestamptz not null default now(),

    primary key (agent_id, version_id)
);

-- A claim is a fact about bytes, like the version it names.
create function agent_version_digests_are_permanent() returns trigger as $$
begin
    raise exception 'agent_version_digests is insert-only: a claim cannot be changed (NT-018)';
end;
$$ language plpgsql;

create trigger agent_version_digests_permanent
    before update or delete on agent_version_digests
    for each row execute function agent_version_digests_are_permanent();

-- How an insert presents its digest. Not a second record: the claim is the
-- record, and release C2's trigger requires the two to be equal. A row written
-- before this column, or by a binary that does not know it, leaves it NULL.
alter table agent_specs add column digest text;

-- Every version already published is claimed without a digest.
insert into agent_version_digests (agent_id, version_id, digest)
select agent_id, version_id, null from agent_specs
on conflict (agent_id, version_id) do nothing;
