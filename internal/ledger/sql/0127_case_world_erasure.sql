-- NT-018 phase 5b: a case's world follows its input when the input is erased.
--
-- A case kept from a rehearsal holds, beside its input, the answers the author gave and the
-- object naming them, all owned by the case (`<agent>/<case>`, kind case_world). They came
-- from the same rehearsal as the input, so an erasure that reaches the input reaches them:
-- whether it is a data subject's, retention's, or anything else that erases the copy. The
-- answers are never found by digest, as copies are: an answer that happens to equal another
-- run's input is not that input, and erasing that run leaves it.
--
-- Statement-level, beside 0102's trigger on the same update: one pass over what changed.
-- The update it makes erases only case_world rows, which it then ignores.
create function erase_worlds_of_erased_copies() returns trigger
language plpgsql as $$
begin
    if not exists (select 1 from changed
                   where erased_at is not null and owner_kind = 'regression') then
        return null;
    end if;

    update run_content held
    set bytes = null, erased_at = now(), erased_reason = owners.reason
    from (
        select c.agent_id || '/' || c.case_id as owner, changed.erased_reason as reason
        from changed
        join regression_cases c on c.input_ref = changed.ref
        where changed.erased_at is not null and changed.owner_kind = 'regression'
        union
        select d.agent_id || '/' || v.case_id, changed.erased_reason
        from changed
        join dataset_version_cases v on v.input_ref = changed.ref
        join datasets d on d.dataset_id = v.dataset_id
        where changed.erased_at is not null and changed.owner_kind = 'regression'
    ) owners
    where held.owner_kind = 'case_world' and held.run_id = owners.owner
      and held.erased_at is null;

    return null;
end;
$$;

create trigger case_world_follows_erased_copy
    after update on run_content
    referencing new table as changed
    for each statement execute function erase_worlds_of_erased_copies();
