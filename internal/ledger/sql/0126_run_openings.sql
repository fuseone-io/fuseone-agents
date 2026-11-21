-- #432: one idempotency key opens one run, whenever the attempts arrive.
--
-- Since 0026 the idempotency index leads with the partition key, (opened_at, idem_key): two
-- calls of one run collide on it, which is what it is for. Two openings under one key do not.
-- Each derives its run from the moment it was attempted, so two attempts a millisecond apart
-- carry different opened_at, and both were sealed: one key, two runs, each with its input.
--
-- An opening claims its key here, in the transaction that inserts it, whatever binary wrote
-- it: the trigger is the database's, so a release from before this one is held to it too. A
-- second opening under a claimed key fails on this table's key, whose name carries
-- "idem_key", which every binary already reads as an idempotency conflict and answers with
-- the run the key names. A table of keys and ids only: the key is what run_steps already
-- holds, and nothing anybody typed is added.
create table run_openings (
    idem_key text not null,
    run_id   text not null,
    constraint run_openings_idem_key_uniq primary key (idem_key)
);

-- Keys already used name the run that opened first; a key two runs already share keeps the
-- earlier, and the later one stays as history.
insert into run_openings (idem_key, run_id)
select distinct on (idem_key) idem_key, run_id
from run_steps
where kind = 'run_started' and idem_key <> ''
order by idem_key, at, run_id
on conflict (idem_key) do nothing;

create function claim_run_opening() returns trigger
language plpgsql as $$
begin
    insert into run_openings (idem_key, run_id) values (new.idem_key, new.run_id);
    return new;
end;
$$;

create trigger run_steps_claim_opening
    before insert on run_steps
    for each row
    when (new.kind = 'run_started' and new.idem_key <> '')
    execute function claim_run_opening();
