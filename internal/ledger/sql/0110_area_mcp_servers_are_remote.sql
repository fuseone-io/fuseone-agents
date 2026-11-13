-- An area's or a company's MCP server is remote (NT-019, decided 2026-11-13).
--
-- A stdio server is a process the platform's worker starts on its own host:
-- its command, arguments, environment and configuration file run there.
-- Registered by an area, it would run that area's commands on infrastructure
-- every area shares, so only the installation holds one. Remote servers reach
-- the network through the worker's guard against internal addresses.
--
-- Held by the database, so the rule binds every binary, the previous one
-- included. An absent transport reads as stdio, as rows from before the field
-- always have, so the rule asks for http by name. Coalesced: a CHECK passes on
-- NULL, and an absent field compared with anything is NULL.
alter table settings add constraint settings_area_mcp_servers_are_remote
    check (kind <> 'mcp_server' or scope_kind = 'installation'
           or coalesce(value->>'transport', '') = 'http');
