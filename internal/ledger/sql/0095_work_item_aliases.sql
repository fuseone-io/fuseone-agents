-- The pull requests linked to the card a run claimed, through
-- $fuseone.work_item.link (#204, slice 2b).
--
-- A projection of the work_item_linked step, written in the same transaction
-- that seals it, so the ledger and this table never disagree. It is what joins
-- a card's trail to its pull requests: a run that claimed the alias is on the
-- root's trail.
--
-- Keyed per company, as every other record is: a reference is a name inside
-- one company's work, and two companies on one repository keep their own
-- links. The primary key is "one root per alias". "One level deep" — a root
-- is never an alias, an alias never a root — spans rows, so the append judges
-- it instead, under advisory locks on both references taken in sorted order
-- before anything is read; see ledger.recordWorkItemLink.
--
-- Who and where are the linking run's, from the runs projection: never the
-- step's own stamp. Simulated runs project nothing.
--
-- A link is never amended and never removed: corrections would be new steps,
-- and there are none to write, because an alias has one root for good. So the
-- rows are append-only, like the steps they project.
--
-- The trail reads it by alias (resolving the reference asked for) and by root
-- (gathering the aliases), with no company in hand: the caller's grants may
-- span companies, and there is at most one row per company for a reference.
-- Postgres 18 reaches both through the leading company column by skip scan.
create table work_item_aliases (
    company_id text        not null,
    ref        text        not null,
    root_ref   text        not null,
    area_id    text        not null,
    agent_id   text        not null,
    run_id     text        not null,
    linked_seq bigint      not null,
    linked_at  timestamptz not null,
    primary key (company_id, ref)
);

-- The aliases of a root: the trail, and the "one level deep" judgement.
create index work_item_aliases_root_idx on work_item_aliases (company_id, root_ref);
-- The links a run made.
create index work_item_aliases_run_idx on work_item_aliases (run_id);

create or replace function work_item_aliases_append_only() returns trigger
language plpgsql as $$
begin
    raise exception 'work_item_aliases is append-only: % rejected on %',
        lower(tg_op), old.ref
        using errcode = 'restrict_violation';
end;
$$;

create trigger work_item_aliases_no_update
    before update or delete on work_item_aliases
    for each row execute function work_item_aliases_append_only();
