-- Declared areas.
--
-- An area used to exist only as text typed into an agent, which made
-- `financeiro` and `Financeiro` two areas that never meet: a ceiling set on one
-- governs no agent filed under the other, and nothing in the product reports
-- it. This table is what makes the set of areas something the platform knows
-- rather than something it reconstructs from whatever rows mention one.
--
-- The company itself is not a row here. Every caller's grants already name the
-- companies they reach, and a registry that also had to be kept in step with
-- them would be a second answer to a question that already has one.
create table if not exists scopes (
    company_id text        not null,
    area_id    text        not null,
    label      text        not null default '',
    created_at timestamptz not null default now(),
    created_by text        not null default '',
    primary key (company_id, area_id)
);
