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

-- When a sweep first saw an unmanaged server published the old way.
--
-- A worker from before this migration publishes --mcp servers marked
-- unmanaged and nothing else, and never renews a lease it does not know
-- exists. During a rolling upgrade it may still be serving them while a new
-- worker, without the flag, sweeps. Its rows are told apart by carrying no
-- lease marker (the new workers write one), and each such server is given
-- one grace period counted from the first sweep that saw it, not from this
-- migration: an old worker can publish a server for the first time an hour
-- into the rollout, and its grace starts then. The row goes when the server
-- has no old-style rows left, so an old worker publishing it again later
-- gets a grace of its own.
create table tool_server_grace (
    server     text        primary key,
    first_seen timestamptz not null
);
