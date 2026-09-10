-- A conversation belongs to a connection, and the key says so.
--
-- Conversations were stored under the vendor's conversation id alone, with the
-- connection named only inside the value. Slack channel ids and Teams
-- conversation ids are two namespaces and nothing promises they never collide,
-- which is why the runtime has always resolved by connection *and* id — the
-- storage was the half that did not, so mapping the same id on a second
-- workspace silently replaced the first.
--
-- The second half of a two-release move. The release before this one learned to
-- read both shapes and went on writing the old one, because both versions serve
-- during a rolling update and a key that moved under the older one would take
-- every conversation away from it. That release is now what a rollout leaves
-- behind, so the rename is safe.
--
-- Three things this statement has to get right, each of which was got wrong on
-- the way here:
--
--   * `keyVersion` is set in the same statement as the rename. A renamed row
--     that does not declare the new shape is read as an id containing a colon
--     and a slash — which is nobody's conversation.
--   * The prefix is compared with `left`, not `like`: a connection named with a
--     `%` or a `_` would otherwise match rows it has nothing to do with.
--   * The length is in octets, because `ConversationKey` counts bytes and
--     connection names accept Unicode.
--
-- A rename that would land on an existing row is skipped rather than allowed to
-- abort the migration: a migration that dies is a process that will not start,
-- and a row already in the new shape is a conversation somebody has since
-- saved.
-- The connections whose conversations are about to move, locked first.
--
-- This runs as a pre-upgrade hook, so the release before it is still serving —
-- and that release reads both shapes and writes the old one. Without the lock
-- the order that happens is: a pod decides to write, this statement renames the
-- row, the pod writes the old name back, and the conversation exists twice. The
-- runtime then refuses it as ambiguous, which is a conversation broken by an
-- upgrade that reported success.
--
-- The same lock `PutConversation` takes, by the same name, so the two queue
-- rather than interleave. Taken in one deterministic order, because a migration
-- that deadlocks with a worker is a migration that fails.
select pg_advisory_xact_lock(hashtext('channel:' || connection))
  from (select distinct settings.value->>'channel' as connection
          from settings
         where settings.kind = 'channel_conversation'
           and coalesce(settings.value->>'channel', '') <> ''
         order by 1) as affected;

update settings as target
   set name = octet_length(target.value->>'channel')::text || ':'
              || (target.value->>'channel') || '/' || target.name,
       value = jsonb_set(target.value, '{keyVersion}', '2'::jsonb, true)
 where target.kind = 'channel_conversation'
   and coalesce(target.value->>'channel', '') <> ''
   -- The legacy shape is the absence of the field, recognised without reading
   -- what is there. Cast to an integer, a restored row carrying "broken" or a
   -- boolean aborts the statement — and with it the hook, and with that the
   -- upgrade: one unreadable row stopping an installation from starting, where
   -- the reader that meets the same row simply calls it nobody's conversation.
   and target.value->'keyVersion' is null
   and not exists (
       select 1 from settings as taken
        where taken.scope_kind = target.scope_kind
          and taken.company_id = target.company_id
          and taken.area_id = target.area_id
          and taken.kind = target.kind
          and taken.name = octet_length(target.value->>'channel')::text || ':'
                           || (target.value->>'channel') || '/' || target.name);
