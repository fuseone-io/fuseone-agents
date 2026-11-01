-- Who still serves a tool server the configuration does not hold (#223).
--
-- A server started with --mcp is published as unmanaged: the configuration
-- has no row to judge it by, so the sweep that clears the tools of removed
-- servers leaves it to the worker that started it. Nothing ever took its
-- tools away once every worker restarted without the flag.
--
-- Each worker that runs such a server holds a lease on it here, renewed on
-- every reconcile pass. A publication leases the unmanaged servers it speaks
-- for, and the sweep drops the tools of an unmanaged server nobody holds a
-- live lease on. One row per server and holder, so a worker that drops the
-- flag cannot take away tools another live worker still serves: the server
-- leaves only when the last lease on it runs out.
--
-- Rulings are not catalogue rows, and nothing here touches them.
create table tool_server_leases (
    server     text        not null,
    holder     text        not null,
    expires_at timestamptz not null,
    primary key (server, holder)
);
