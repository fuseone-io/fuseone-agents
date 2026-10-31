-- The card each run claimed through $fuseone.work_item.claim (#206, NT-013).
--
-- A projection of the work_item_claimed step, written in the same transaction
-- that seals it, so the ledger and this table never disagree. It is the key
-- FinOps and the trail join on to say what a card cost, runs that failed
-- before their first write included.
--
-- One row per run: the primary key is "one card per run". "One open run per
-- card per agent" cannot be a constraint, because open is the run's phase and
-- the phase moves on runs, not here. The append judges it instead, under an
-- advisory lock on (agent, ref) taken before the judgement is read — see
-- ledger.recordWorkItemClaim.
--
-- Nothing releases a claim. A run holds its card while runs.phase is not
-- terminal and stops holding it the moment the run finishes or fails, however
-- that happens — a crash cannot leave an orphan behind, because there is no
-- release to miss. So the rows are append-only, like the steps they project.
create table run_work_items (
    run_id      text        not null primary key,
    company_id  text        not null,
    area_id     text        not null,
    agent_id    text        not null,
    ref         text        not null,
    claimed_seq bigint      not null,
    claimed_at  timestamptz not null
);

-- The judgement: other runs of this agent holding this card.
create index run_work_items_agent_ref_idx on run_work_items (agent_id, ref);
-- Cost per card, across every agent that worked on it.
create index run_work_items_ref_idx on run_work_items (ref);

create or replace function run_work_items_append_only() returns trigger
language plpgsql as $$
begin
    raise exception 'run_work_items is append-only: % rejected on run %',
        lower(tg_op), old.run_id
        using errcode = 'restrict_violation';
end;
$$;

create trigger run_work_items_no_update
    before update or delete on run_work_items
    for each row execute function run_work_items_append_only();
