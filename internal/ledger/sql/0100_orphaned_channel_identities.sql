-- Bindings left behind by connections that were deleted.
--
-- Deleting a connection removed its conversations and kept who each account spoke for.
-- A new connection taking the same name would have adopted them, handing somebody's
-- authority to whoever holds that account there, with nobody having decided it. From
-- here on a delete takes them. These are the ones already left behind, and each goes
-- on the trail with who it spoke for, so it can be bound again on purpose.
with gone as (
    delete from settings s
    where s.kind = 'channel_identity'
      and not exists (
        select 1 from settings c
        where c.scope_kind = 'installation' and c.kind = 'channel'
          and c.name = split_part(s.name, '/', 1)
      )
    returning s.name, s.value
)
insert into admin_events (principal_id, action, target, detail)
select '', 'channel.identity.unbound', gone.name,
       jsonb_build_object('reason', 'connection no longer exists', 'principal', gone.value->>'principal')
from gone;
