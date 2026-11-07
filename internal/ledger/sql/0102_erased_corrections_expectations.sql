-- What a correction expected, on corrections already erased.
--
-- Erasure and retention used to clear a correction's note and leave its
-- expectations, which are free text about the same occurrence: nothing validates
-- Expectation.Value. They clear both now, and this is the ones they already passed.
-- A later step turns expectations into content, and these must not arrive readable.
--
-- Only where the input is gone. Everywhere else, the case still runs and needs them.
update regression_cases c
set expectations = '[]'::jsonb
from run_content rc
where rc.ref = c.input_ref
  and rc.erased_at is not null
  and c.expectations <> '[]'::jsonb;
