-- The regression corpus: corrections an author made, kept to be re-run.
--
-- A correction is only worth recording if a future version can be checked
-- against it without a person reading the run again (FU-12), so what is stored
-- is the occurrence plus the expectations — never prose about it.
--
-- The occurrence is copied into the corpus rather than pointed at inside the
-- run it came from. Runs are purged on the installation's retention (AU-11),
-- and a corpus that lost its cases when a run aged out would quietly stop
-- checking anything — the worst possible failure for a safety net, because it
-- keeps reporting green.
create table regression_cases (
    agent_id     text        not null,
    case_id      text        not null,
    company_id   text        not null default '',
    area_id      text        not null default '',

    -- Where the occurrence lives in the claim check, under the corpus.
    input_ref    text        not null,
    -- What must be true of it. Checked against the fold of a simulated run.
    expectations jsonb       not null default '[]'::jsonb,

    -- The run the correction was made from, so somebody can go and read what
    -- prompted it. It may be purged long before the case is.
    from_run     text        not null default '',
    note         text        not null default '',

    created_by   text        not null default '',
    created_at   timestamptz not null default now(),

    primary key (agent_id, case_id)
);

-- The battery reads the whole corpus of one agent, oldest first: a case list
-- that reshuffles between runs makes two reports impossible to compare.
create index regression_cases_agent_idx on regression_cases (agent_id, created_at, case_id);
