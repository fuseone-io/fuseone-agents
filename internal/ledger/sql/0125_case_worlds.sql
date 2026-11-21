-- NT-018 phase 5: a case keeps the world it was corrected in.
--
-- The world is one content object, of its own kind and owned by the case, naming the
-- answers an author gave in a rehearsal in the order they were asked. The case points at it
-- as it points at its expectations, and a version records it by reference and digest beside
-- them. A case with no world names none: the empty default, which is what every case before
-- this migration is, and what a binary before it writes.
alter table regression_cases add column world_ref text not null default '';
alter table dataset_version_cases add column world_ref text not null default '';
alter table dataset_version_cases add column world_digest text not null default '';

-- Only a writer that records versions may move a case's world. The rule 0104 gives the
-- working set is restated with the new column: an undeclared writer may still clear
-- expectations after an erasure, and nothing else.
create or replace function refuse_undeclared_corpus_writes() returns trigger
language plpgsql as $$
begin
    if current_setting('fuseone.dataset_writer', true) = 'version-aware' then
        return coalesce(new, old);
    end if;
    if tg_op = 'UPDATE'
       and new.agent_id = old.agent_id and new.case_id = old.case_id
       and new.input_ref = old.input_ref and new.from_run = old.from_run
       and new.world_ref = old.world_ref
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
