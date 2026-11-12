-- The database enforces version claims (NT-018, release C2).
--
-- Release C1 claims every version it writes, in the binary. The previous
-- release's replicas do not, and they keep serving through a rolling update
-- and come back on a rollback. This migration runs only once C1 is on every
-- replica, and from here on a write that cannot prove its digest is refused
-- by the database, whatever binary sends it.
--
-- Rollback floor: once this has run, rolling back below C1 is not supported.
-- A release without claims has every publication refused. That fails closed,
-- with no wrong row written, but publishing stops until C1 or later is back.

-- 1. The gap. A row a C0 replica wrote during C1's rollout has no claim, and
-- its bytes were never kept. It is claimed without a digest, like every
-- version C1's migration found.
insert into agent_version_digests (agent_id, version_id, digest)
select agent_id, version_id, null from agent_specs
on conflict (agent_id, version_id) do nothing;

-- 2. An insert presents the digest it claimed. A BEFORE INSERT row trigger
-- fires before ON CONFLICT looks for the conflict, so a C0 insert it would have
-- answered with "do nothing" over another definition's row is refused too, and
-- the transaction that would have made that row current aborts with it.
create function agent_specs_present_their_claim() returns trigger as $$
begin
    if new.digest is null then
        raise exception 'agent_specs: % % presents no digest; claim it first (NT-018)',
            new.agent_id, new.version_id;
    end if;
    if not exists (
        select 1 from agent_version_digests c
        where c.agent_id = new.agent_id and c.version_id = new.version_id
          and c.digest = new.digest
    ) then
        raise exception 'agent_specs: % % presents a digest no claim holds (NT-018)',
            new.agent_id, new.version_id;
    end if;
    return new;
end;
$$ language plpgsql;

create trigger agent_specs_claimed
    before insert on agent_specs
    for each row execute function agent_specs_present_their_claim();

-- 3. The current version has a proven digest.
--
-- Two guards keep what must keep working. State without a version (pausing,
-- stages, retirement, a publication's state row) sets no current version. And
-- naming the version that is already current changes nothing, so every legacy
-- version current today stays current.
--
-- makeCurrentOn is INSERT ... ON CONFLICT DO UPDATE, and the BEFORE INSERT
-- trigger fires on the proposed row even when the row exists. So the insert
-- side accepts the value that is already current, and the update side, which
-- sees OLD, judges any change.
create function agent_state_current_is_proven() returns trigger as $$
begin
    if new.current_version is null then
        return new;
    end if;
    if tg_op = 'UPDATE' and new.current_version is not distinct from old.current_version then
        return new;
    end if;
    if tg_op = 'INSERT' and exists (
        select 1 from agent_state s
        where s.agent_id = new.agent_id and s.current_version = new.current_version
    ) then
        return new;
    end if;
    if not exists (
        select 1 from agent_version_digests c
        where c.agent_id = new.agent_id and c.version_id = new.current_version
          and c.digest is not null
    ) then
        raise exception 'agent_state: % cannot make % current: no proven digest (NT-018)',
            new.agent_id, new.current_version;
    end if;
    return new;
end;
$$ language plpgsql;

create trigger agent_state_current_proven
    before insert or update of current_version on agent_state
    for each row execute function agent_state_current_is_proven();
