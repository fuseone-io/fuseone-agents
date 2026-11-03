-- The card each run inherited by how it was opened (#204, slice 3).
--
-- A projection of run_started.work_item, written in the same transaction that
-- seals the run's opening step, so the ledger and this table never disagree.
-- An event-opened run inherits the card its source run claimed, or the one the
-- source itself inherited; a run a signed GitHub delivery opened inherits the
-- issue or pull request the delivery is about.
--
-- Attribution only, and kept apart from run_work_items on purpose: that table
-- is what a claim is judged against, and nothing here may ever hold a card or
-- block a claim. A run that inherited a card may claim it, or another, as if it
-- had inherited nothing. Only the trail reads this table.
--
-- One row per run, written from the run's opening step, whose agent and scope
-- are the run's own. Simulated runs project nothing.
--
-- An inheritance is never amended and never removed: it is what opened the
-- run. So the rows are append-only, like the steps they project.
create table run_work_item_origins (
    run_id     text        not null primary key,
    company_id text        not null,
    area_id    text        not null,
    agent_id   text        not null,
    ref        text        not null,
    source     text        not null check (source in ('event', 'github_delivery')),
    from_run   text,
    started_at timestamptz not null,
    check ((source = 'event') = (from_run is not null))
);

-- The trail reads a card's inherited runs by reference, as it reads its claims
-- by run_work_items_ref_idx.
create index run_work_item_origins_ref_idx on run_work_item_origins (ref);

create or replace function run_work_item_origins_append_only() returns trigger
language plpgsql as $$
begin
    raise exception 'run_work_item_origins is append-only: % rejected on run %',
        lower(tg_op), old.run_id
        using errcode = 'restrict_violation';
end;
$$;

create trigger run_work_item_origins_no_update
    before update or delete on run_work_item_origins
    for each row execute function run_work_item_origins_append_only();
