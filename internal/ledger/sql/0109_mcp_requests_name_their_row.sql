-- A rediscovery request and its receipts name the server row they are about
-- (NT-019).
--
-- A server's name is unique, but not forever: a server removed from one area
-- and stored again under the same name in another is a different server. A
-- request asked of the first must not be answered by the second, and the
-- second's administrator must not be shown the first's request or receipts.
-- So both keep where the server they are about was stored.
--
-- Defaults are the installation: every request and receipt before this was
-- about a server of the installation, the only place servers were stored.
alter table mcp_rediscoveries
    add column scope_kind text not null default 'installation',
    add column company_id text not null default '',
    add column area_id    text not null default '';

alter table mcp_rediscovery_receipts
    add column scope_kind text not null default 'installation',
    add column company_id text not null default '',
    add column area_id    text not null default '';
