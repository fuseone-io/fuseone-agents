-- A binding always has its connection, enforced where every writer has to pass.
--
-- Deleting a connection used to keep who each account spoke for, so a new connection
-- taking the name adopted them, handing somebody's authority to whoever holds that
-- account there, with nobody having decided it.
--
-- Here rather than only in the code, because an upgrade is rolling: a replica still on
-- the previous version writes against this schema and knows none of the new locks, and
-- a cleanup that ran once would be followed by the next orphan. Both writers take the
-- connection's own row, so they serialise on it whatever version they run.

-- The last slash separates them. A connection's name may hold one, an account never does.
create or replace function channel_identity_connection(key text) returns text
language sql immutable as $$
    select case when position('/' in key) = 0 then key
                else left(key, length(key) - position('/' in reverse(key))) end
$$;

create or replace function settings_binding_needs_connection() returns trigger
language plpgsql as $$
begin
    perform 1 from settings c
    where c.scope_kind = 'installation' and c.kind = 'channel'
      and c.name = channel_identity_connection(new.name)
    for share;
    if not found then
        raise exception 'no connection for the binding %', new.name using errcode = '23503';
    end if;
    return new;
end;
$$;

create or replace function settings_connection_keeps_bindings() returns trigger
language plpgsql as $$
begin
    -- Editing a connection is not removing it. Only its identity going counts.
    if tg_op = 'UPDATE' and new.scope_kind = old.scope_kind and new.kind = old.kind
       and new.name = old.name then
        return new;
    end if;
    if exists (
        select 1 from settings b
        where b.kind = 'channel_identity' and channel_identity_connection(b.name) = old.name
    ) then
        raise exception 'the connection % still has bindings', old.name using errcode = '23503';
    end if;
    if tg_op = 'DELETE' then
        return old;
    end if;
    return new;
end;
$$;

drop trigger if exists settings_binding_needs_connection on settings;
create trigger settings_binding_needs_connection
    before insert or update on settings
    for each row when (new.kind = 'channel_identity')
    execute function settings_binding_needs_connection();

drop trigger if exists settings_connection_keeps_bindings on settings;
create trigger settings_connection_keeps_bindings
    before delete or update on settings
    for each row when (old.kind = 'channel' and old.scope_kind = 'installation')
    execute function settings_connection_keeps_bindings();

-- The rules first, in this transaction, so nothing can be left behind after the cleanup.
-- Each one goes on the trail with who it spoke for, so it can be bound again on purpose.
with gone as (
    delete from settings s
    where s.kind = 'channel_identity'
      and not exists (
        select 1 from settings c
        where c.scope_kind = 'installation' and c.kind = 'channel'
          and c.name = channel_identity_connection(s.name)
      )
    returning s.name, s.value
)
insert into admin_events (principal_id, action, target, detail)
select '', 'channel.identity.unbound', gone.name,
       jsonb_build_object('reason', 'connection no longer exists', 'principal', gone.value->>'principal')
from gone;
