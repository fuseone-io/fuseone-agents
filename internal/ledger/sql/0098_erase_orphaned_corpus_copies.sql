-- Corpus copies nothing points at any more.
--
-- Until now, deleting a correction left its copy of the occurrence in the claim
-- check, filed under the agent. With no case naming it, no erasure could find
-- it: following a run to its corrections stops at the row that was deleted.
-- From here on a delete takes the copy with it. This is the one time the ones
-- already left behind are taken.
--
-- Migration 0018 says the copy exists so the corpus survives retention. That is
-- not what happens and not what was decided: a copy ages out like any other
-- content, and the case stays as a visible gap (DP-001 section 4).
with erased as (
    update run_content rc
    set bytes = null, erased_at = now(), erased_reason = 'case_deleted'
    where rc.owner_kind = 'regression'
      and rc.erased_at is null
      and not exists (
        select 1 from regression_cases c where c.input_ref = rc.ref
      )
    returning rc.ref
)
-- An erasure nobody can account for is indistinguishable from data loss.
insert into admin_events (principal_id, action, target, detail)
select '', 'content.erased', 'content',
       jsonb_build_object('objects', count(*), 'reason', 'orphaned corpus copies')
from erased
having count(*) > 0;
