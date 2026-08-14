-- What a company is called, beside what it is keyed by.
--
-- The identifier reaches a URL, a settings key and every scope written as
-- "company/area", so it is lowercase, hyphenated and never changes. The label
-- is what people read, and people rename things — a company that had to be
-- re-keyed to be renamed would take every run, grant and policy that names it
-- along with it.
--
-- `name` already existed and was the identifier repeated, because nothing
-- wrote anything else to it. It becomes the label.
alter table companies
    add column if not exists created_by text not null default '';

-- Archived rather than deleted, and the column was there from the start. A
-- company with runs against it cannot be removed without removing the record
-- of what those runs did, so withdrawing one takes it out of what is offered
-- for new work and leaves the history readable.
create index if not exists companies_live_idx
    on companies (company_id) where archived_at is null;
