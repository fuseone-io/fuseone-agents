-- A conversation belongs to a connection, and the key has to say so.
--
-- Conversations were stored under the vendor's conversation id alone, with the
-- connection named only inside the value. Slack channel ids and Teams
-- conversation ids are two namespaces and nothing promises they never collide,
-- which is why the runtime resolves by connection *and* id — but two rows for
-- one scope could not both exist, so mapping the same id on a second workspace
-- silently replaced the first. No refusal, no record of a removal: cards and
-- approvals for that workspace stopped.
--
-- The name becomes "<connection>/<conversation>". Rows already carrying their
-- connection are left alone so this is safe to run twice, and a row whose value
-- names no connection is left as it is: it resolves to nothing today and
-- renaming it would only invent a connection for it.
-- A rename that would land on an existing row is skipped rather than allowed to
-- abort the migration. It cannot happen on the way in — before this file the
-- key had no room for a connection — and a migration that dies is a process
-- that will not start, which is a worse answer than one row left as it was.
update settings as target
   set name = (target.value->>'channel') || '/' || target.name
 where target.kind = 'channel_conversation'
   and coalesce(target.value->>'channel', '') <> ''
   and target.name not like (target.value->>'channel') || '/%'
   and not exists (
       select 1 from settings as taken
        where taken.scope_kind = target.scope_kind
          and taken.company_id = target.company_id
          and taken.area_id = target.area_id
          and taken.kind = target.kind
          and taken.name = (target.value->>'channel') || '/' || target.name);
