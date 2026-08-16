-- A conversation id belongs to the connection it came from.
--
-- The table keyed delivery on `(run_id, event, conversation)`, which reads as
-- unique and is not: two workspaces are two namespaces, and nothing said a
-- Slack channel id could not also be a Teams conversation id. With two
-- connections configured, a reply in one could resolve to a run reported in
-- the other — a channel somebody cannot see naming a run they cannot read.
--
-- Nobody has two connections yet, which is why this is cheap today and would
-- not have been in a year.
alter table channel_deliveries
    add column if not exists channel text not null default '';

alter table channel_deliveries
    drop constraint if exists channel_deliveries_pkey;

alter table channel_deliveries
    add primary key (run_id, event, channel, conversation);

-- Resolving a thread back to the run it is about, which is the boundary of
-- what the platform is entitled to claim it knows: it resolves references to
-- what it put there, and this is the record of what it put.
create index if not exists channel_deliveries_ref_idx
    on channel_deliveries (channel, conversation, ref);

-- Existing rows keep an empty channel. They are not mistaken for the sentinel
-- that marks a run said everywhere, because that one has an empty conversation
-- too and a real delivery never does.
