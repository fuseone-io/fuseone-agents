-- Dataset versions turn on (NT-015, Rollout). From here every change to a corpus
-- records a version, which is only trustworthy if every write produced one, so this
-- needs the release before it, whose writers already know how.

-- The switch first: a writer in flight holds it shared and finishes first, and the
-- next one waits here and then reads it on. The table lock is for whatever does not
-- read the switch, which is an older pod and an erasure.
select active from dataset_versioning where only_row for update;
lock table regression_cases in share row exclusive mode;

-- One object per case, naming the case, so that erasing one does not leave another
-- case with identical expectations as a gap. It takes the age of the input, so this
-- does not extend how long it lives, and a case whose input is gone becomes a
-- tombstone: 0102 emptied its expectations, and they never become readable here.
with bodies as (
    select c.agent_id, c.case_id, i.created_at, i.erased_at, i.erased_reason,
           convert_to(jsonb_build_object('case', c.case_id, 'expectations', c.expectations)::text,
                      'UTF8') as body
    from regression_cases c
    left join run_content i on i.ref = c.input_ref
), objects as (
    select *, encode(sha256(body), 'hex') as digest from bodies
), stored as (
    insert into run_content
        (ref, owner_kind, run_id, seq, digest, bytes, full_bytes, truncated,
         created_at, erased_at, erased_reason)
    select 'expectations://' || agent_id || '/1/' || left(digest, 16),
           'expectations', agent_id, 1, digest,
           case when erased_at is null then body end, octet_length(body), false,
           coalesce(created_at, now()), erased_at, coalesce(erased_reason, '')
    from objects
    on conflict (ref) do nothing
)
update regression_cases c
set expectations_ref = 'expectations://' || o.agent_id || '/1/' || left(o.digest, 16)
from objects o
where o.agent_id = c.agent_id and o.case_id = c.case_id;

insert into datasets (dataset_id, agent_id, name, company_id, area_id)
select 'default:' || agent_id, agent_id, 'default', min(company_id), min(area_id)
from regression_cases
group by agent_id;

insert into dataset_versions (dataset_id, version, digest, created_by)
select 'default:' || c.agent_id, 1,
       encode(sha256(convert_to(string_agg(
           octet_length(c.case_id) || ':' || c.case_id || ' ' ||
           coalesce(i.digest, c.input_ref) || ' ' || e.digest || E'\n',
           '' order by c.case_id collate "C"), 'UTF8')), 'hex'),
       'system:dataset-activation'
from regression_cases c
left join run_content i on i.ref = c.input_ref
join run_content e on e.ref = c.expectations_ref
group by c.agent_id;

insert into dataset_version_cases
    (dataset_id, version, case_id, input_ref, input_digest,
     expectations_ref, expectations_digest, from_run)
select 'default:' || c.agent_id, 1, c.case_id, c.input_ref,
       coalesce(i.digest, c.input_ref), c.expectations_ref, e.digest, c.from_run
from regression_cases c
left join run_content i on i.ref = c.input_ref
join run_content e on e.ref = c.expectations_ref;

update dataset_versioning set active = true, activated_at = now() where only_row;

-- An older pod, or an installation that skipped release A, fails here without
-- changing the working set, rather than writing a state no version records.
--
-- What erasure does passes: a note is outside a version, and expectations may only be
-- emptied once there is no readable input left, or their object is gone, which is what
-- 0102 and every binary's erasure do. A case whose copy was never stored counts: an
-- erasure naming its run still has to clear it. That write changes no digest, so no
-- version has to record it.
create function refuse_undeclared_corpus_writes() returns trigger
language plpgsql as $$
begin
    if current_setting('fuseone.dataset_writer', true) = 'version-aware' then
        return coalesce(new, old);
    end if;
    if tg_op = 'UPDATE'
       and new.agent_id = old.agent_id and new.case_id = old.case_id
       and new.input_ref = old.input_ref and new.from_run = old.from_run
       and new.expectations_ref is not distinct from old.expectations_ref
       and (new.expectations = old.expectations
            or (new.expectations = '[]'::jsonb and (
                  not exists (select 1 from run_content rc
                              where rc.ref = old.input_ref and rc.erased_at is null)
                  or exists (select 1 from run_content rc
                             where rc.ref = old.expectations_ref and rc.erased_at is not null)))) then
        return new;
    end if;
    raise exception 'regression_cases: % refused, this writer does not record dataset versions', tg_op
        using errcode = 'restrict_violation';
end;
$$;

create trigger regression_cases_versioned_writes
    before insert or update or delete on regression_cases
    for each row execute function refuse_undeclared_corpus_writes();
