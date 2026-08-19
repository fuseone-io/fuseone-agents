-- A hint for administrators binding Slack users to platform principals.
--
-- This is not authority. The authority remains channel_identity; this table is
-- only the last time a signed Slack event showed that account interacting with
-- this installation. It deliberately carries no message text.
create table if not exists channel_accounts_seen (
    channel text not null,
    account text not null,
    conversation text not null default '',
    last_seen timestamptz not null default now(),
    primary key (channel, account)
);

create index if not exists channel_accounts_seen_recent_idx
    on channel_accounts_seen (channel, last_seen desc);
