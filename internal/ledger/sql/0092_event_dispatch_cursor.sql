-- Where the event dispatcher has got to among finished runs (#203).
--
-- It used to read the thirty newest finished runs on every sweep, with no
-- memory of where it had got to: the thirty-first run to finish between two
-- sweeps was never dispatched. Now it reads forward from a cursor and moves
-- the cursor only once a page is dispatched.
--
-- The order is the order runs committed in, not a clock. A run's finish is
-- stamped by the worker before its transaction commits; a run stamped earlier
-- can commit later, and a cursor that had passed its timestamp would never see
-- it. So the projection records the finishing transaction (finished_xid), and
-- the dispatcher reads only below the oldest transaction still in flight
-- (pg_snapshot_xmin): everything there has committed or never will, so no run
-- can later appear behind the cursor. A long transaction elsewhere delays
-- dispatch while it is open; it cannot make the dispatcher skip a run.
--
-- Numbered 0092 because #226 takes 0091; whichever merges second renumbers.

alter table runs add column finished_xid xid8;

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

-- The first deploy. Runs that finished before this column existed have no
-- position, so they are never offered — the history is not replayed. Except
-- the last hour's: the previous dispatcher read only the newest thirty
-- finished runs, so some of those may never have been dispatched. They get
-- this transaction's position, so the new dispatcher offers each of them
-- once. Any that were dispatched already reach the run they opened through
-- the same idempotency key; an hour keeps that re-check cheap.
update runs
   set finished_xid = pg_current_xact_id()
 where phase = 'finished'
   and not simulated
   and ended_at > now() - interval '1 hour';
