-- The planning spend projection is forward-only. The UI needs to say where
-- that forward-only promise begins, otherwise the first days after an upgrade
-- look like the installation spent less than it did.

alter table planning_spend_cursor
    add column if not exists started_at timestamptz;

-- 0.26.0 briefly projected dry-run simulations. The projection is rebuildable
-- by design, so delete those rows here instead of leaving the cost page to
-- report rehearsals as production spend.
delete from planning_spend p
 using runs r
 where r.run_id = p.run_id
   and r.simulated;

update planning_spend_cursor
   set started_at = coalesce(
       (select min(day)::timestamp at time zone 'UTC' from planning_spend),
       scanned_at)
 where started_at is null;

alter table planning_spend_cursor
    alter column started_at set default now(),
    alter column started_at set not null;
