-- Asking every worker to rediscover a tool server's tools (#287).
--
-- Not a settings row, and not a column on the server's configuration: a request
-- changes nothing about the server, so it must not move its revision or its
-- updated_at, either of which would make every worker reconnect and every
-- declarative client read drift. And not the probe queue, which one worker
-- consumes; here every worker answers.
--
-- A request is a generation. Each worker keeps the generation it last answered
-- and owes a rediscovery while it is behind, so one that starts later or missed
-- a pass catches up, and repeated requests fold into the newest.
create table mcp_rediscoveries (
    server       text        primary key,
    generation   bigint      not null check (generation >= 1),
    requested_by text        not null,
    requested_at timestamptz not null default now()
);

-- One row per worker and server: its newest answer. Mutable on purpose; the
-- history of who asked lives on the admin trail.
create table mcp_rediscovery_receipts (
    server     text        not null,
    worker     text        not null,
    generation bigint      not null check (generation >= 0),
    succeeded  boolean     not null,
    at         timestamptz not null default now(),

    primary key (server, worker)
);
