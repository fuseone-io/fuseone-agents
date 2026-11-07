-- What a correction expected goes when its copy goes.
--
-- Erasure and retention used to clear a correction's note and leave its
-- expectations, which are free text about the same occurrence: nothing validates
-- Expectation.Value. Clearing them in the new binary is not enough. During a rolling
-- update the previous one still erases, reports success and leaves them readable,
-- so the rule lives where every erasure passes, whichever binary wrote it.
--
-- For a statement rather than a row, because retention takes many copies at once and
-- this would otherwise look for the corrections of each one. A transition table is
-- not allowed with a column list, so it runs for every update of run_content, and
-- those are erasures and restorations.
create or replace function clear_expectations_of_erased_copies() returns trigger
language plpgsql as $$
begin
    update regression_cases c
    set expectations = '[]'::jsonb
    from changed
    where c.input_ref = changed.ref
      and changed.erased_at is not null
      and c.expectations <> '[]'::jsonb;
    return null;
end;
$$;

-- Before the backfill, on purpose. Creating it waits for every erasure in flight,
-- and the backfill then sees what they committed. The other way round, an erasure
-- committing between the two is seen by neither.
drop trigger if exists corpus_copy_erased on run_content;
create trigger corpus_copy_erased
    after update on run_content
    referencing new table as changed
    for each statement execute function clear_expectations_of_erased_copies();

-- The corrections erased before this. A later step turns expectations into content,
-- and these must not arrive there readable. Only where the input is gone: everywhere
-- else the case still runs and needs them.
update regression_cases c
set expectations = '[]'::jsonb
from run_content rc
where rc.ref = c.input_ref
  and rc.erased_at is not null
  and c.expectations <> '[]'::jsonb;
