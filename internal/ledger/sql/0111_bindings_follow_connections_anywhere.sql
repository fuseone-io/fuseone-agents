-- A binding always has its connection, wherever the connection is stored (NT-019, phase 4).
--
-- 0100 looked for the connection at the installation only, because that was the only
-- place one could be. A company's or an area's connection would refuse every binding
-- as having no connection, and removing it would leave its bindings behind: each one an
-- account still speaking for somebody on whichever connection next takes the name.
--
-- Changing a connection's scope is removing it from where its bindings were made, so it
-- is judged as a removal. A scope is the three columns, not the kind alone: an area's
-- connection handed to a sibling area keeps its kind and changes everything else.
--
-- Less strict than 0100 for a binding and stricter for a removal, so a replica still on
-- the previous version is bound by it too: it writes only installation connections, and
-- those are judged exactly as before.

create or replace function settings_binding_needs_connection() returns trigger
language plpgsql as $$
begin
    perform 1 from settings c
    where c.kind = 'channel' and c.name = channel_identity_connection(new.name)
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
    if tg_op = 'UPDATE' and new.scope_kind = old.scope_kind
       and new.company_id = old.company_id and new.area_id = old.area_id
       and new.kind = old.kind and new.name = old.name then
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

drop trigger if exists settings_connection_keeps_bindings on settings;
create trigger settings_connection_keeps_bindings
    before delete or update on settings
    for each row when (old.kind = 'channel')
    execute function settings_connection_keeps_bindings();
