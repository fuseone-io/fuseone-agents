-- Corpus copies nothing points at any more, and the rehearsals that replayed them.
--
-- Until now, deleting a correction left its copy of the occurrence in the claim
-- check, filed under the agent. With no case naming it, no erasure could find it.
-- Its replays hold the same bytes, and for an input too large to keep whole, nothing
-- else leads to them: the run's digest covers all of it, and theirs covers what was
-- kept. This is the last moment that link exists, so they all go together. From here
-- on a delete takes them itself.
--
-- A tombstoned copy still counts: retention may have taken the copy and left a newer
-- replay. Only inside the agent the copy belongs to, and only into rehearsals.
--
-- Migration 0018 says the copy exists so the corpus survives retention. That is not
-- what happens and not what was decided: a copy ages out like any other content, and
-- the case stays as a visible gap (DP-001 section 4).
with orphans as (
    select rc.ref, rc.digest, rc.run_id as agent_id
    from run_content rc
    where rc.owner_kind = 'regression'
      and not exists (
        select 1 from regression_cases c where c.input_ref = rc.ref
      )
), replays as (
    select distinct held.run_id
    from run_content held
    join orphans o on o.digest = held.digest
    join runs r on r.run_id = held.run_id and r.agent_id = o.agent_id
    where held.owner_kind = 'run' and held.erased_at is null and r.simulated
), erased as (
    update run_content rc
    set bytes = null, erased_at = now(), erased_reason = 'case_deleted'
    where rc.erased_at is null
      and ((rc.owner_kind = 'regression' and rc.ref in (select ref from orphans))
           or (rc.owner_kind = 'run' and rc.run_id in (select run_id from replays)))
    returning rc.ref
)
-- An erasure nobody can account for is indistinguishable from data loss.
insert into admin_events (principal_id, action, target, detail)
select '', 'content.erased', 'content',
       jsonb_build_object('objects', count(*), 'reason', 'orphaned corpus copies')
from erased
having count(*) > 0;
