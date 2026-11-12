-- An integration's name is unique per kind across the installation (NT-019).
--
-- A tool id (server.tool, connector.instance.operation) and the name an
-- inbound message is matched by carry no scope. Two scopes holding one name
-- would be two things answering to one identity. The settings primary key
-- includes the scope, so it cannot say this; a partial unique index can, and
-- binds every binary, the previous one included.
--
-- For connector instances the name alone is unique, not (connector, name): the
-- name alone is already the identity within a scope and in bindings. Two
-- instances of different connectors can no longer share a name in two scopes.
--
-- Kinds that repeat a name across scopes by design stay out: tool rulings (an
-- owner's and the platform's), personal MCP credentials, channel identities,
-- budgets and the rest.

-- A duplicate already stored stops the upgrade before anything changes, and
-- names itself so the platform's administration can rename one of the two.
do $$
declare
    taken text;
begin
    select string_agg(kind || ' ' || name, ', ' order by kind, name) into taken
    from (
        select kind, name from settings
        where kind in ('mcp_server', 'model_provider', 'connector_instance',
                       'channel', 'channel_conversation')
        group by kind, name having count(*) > 1
    ) duplicates;
    if taken is not null then
        raise exception 'settings: integration names held by more than one scope: %. Rename one of each before upgrading (NT-019)', taken;
    end if;
end;
$$;

create unique index settings_integration_names on settings (kind, name)
where kind in ('mcp_server', 'model_provider', 'connector_instance',
               'channel', 'channel_conversation');
