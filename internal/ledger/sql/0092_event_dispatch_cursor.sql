-- Where the event dispatcher has got to among finished runs (#203).
--
-- It used to read the thirty newest finished runs on every sweep, with no
-- memory of where it had got to: the thirty-first run to finish between two
-- sweeps was never dispatched. Now it reads forward from a cursor and moves
-- the cursor only once a page is dispatched.
--
-- Not a clock: a run's finish is stamped by the worker before its
-- transaction commits, so a run stamped earlier can commit later, and a cursor
-- that had passed its timestamp would never see it. The order is the id of the
-- finishing transaction (finished_xid) — the order ids were allocated in,
-- which is not commit order either: a lower id can commit after a higher one.
-- So the dispatcher reads only below the oldest transaction still in flight
-- (pg_snapshot_xmin). Every transaction there has committed or never will, so
-- the order below that boundary is stable and no run can later appear behind
-- the cursor. A long transaction elsewhere delays dispatch while it is open;
-- it cannot make the dispatcher skip a run.
--
-- Cheap on a ledger with years of history: the column is added without a
-- default (no rewrite), and only runs that finish from now on — plus the last
-- hour's — ever get a value, so the partial index holds those and nothing else.
--
-- Numbered 0092 because #226 takes 0091; whichever merges second renumbers.

alter table runs add column finished_xid xid8;

-- Stamped by the database, not by the code that finishes a run. During a
-- rolling update a worker on the previous version finishes runs without
-- knowing this column exists; so does a binary rolled back, or a row inserted
-- straight into its terminal phase. Left NULL, such a run would be invisible
-- to the dispatcher for ever. The trigger is the only writer of the column:
-- one rule, whoever moved the run to 'finished' — the phase the dispatcher
-- reads.
--
-- Only the moment of finishing stamps: a row inserted already finished, or
-- an update that moves a run into 'finished'. A run that finished before this
-- migration keeps no position for ever, so a later update of it — a future
-- migration, a repair — cannot make it look as if it had just finished and
-- dispatch its events years late.
create function runs_stamp_finished_xid() returns trigger
language plpgsql as $$
begin
    if new.phase = 'finished' and new.finished_xid is null
       and (tg_op = 'INSERT' or old.phase is distinct from 'finished') then
        new.finished_xid := pg_current_xact_id();
    end if;
    return new;
end;
$$;

create trigger runs_finished_xid
    before insert or update on runs
    for each row execute function runs_stamp_finished_xid();

create index runs_finished_xid_idx on runs (finished_xid, run_id)
    where finished_xid is not null;

-- One row for the installation: the position every replica reads from. It
-- only moves forward — an advance compares and keeps the furthest — so two
-- replicas finishing pages out of order cannot move it back. No row means the
-- start.
create table event_dispatch_cursor (
    id           boolean primary key default true check (id),
    finished_xid xid8    not null,
    run_id       text    not null
);

-- The first deploy. Runs that finished before this column existed keep no
-- position, so they are never offered — the history is not replayed. Except
-- the last hour's: the previous dispatcher read only the newest thirty
-- finished runs, so some of those may never have been dispatched. They get
-- this transaction's position, so the new dispatcher offers each of them
-- once. Any that were dispatched already reach the run they opened through
-- the same idempotency key; an hour keeps that re-check cheap.
--
-- A function so the rule is one statement in one place, called once here.
create function runs_backfill_finished_xid() returns void
language sql as $$
    update runs
       set finished_xid = pg_current_xact_id()
     where phase = 'finished'
       and not simulated
       and finished_xid is null
       and ended_at > now() - interval '1 hour';
$$;

select runs_backfill_finished_xid();
