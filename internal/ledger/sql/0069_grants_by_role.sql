-- The reverse question: who holds this role here.
--
-- Every lookup so far started from a principal — what may this person do — or
-- from a scope. Asking somebody to decide starts from a role instead, on every
-- announcement sweep, for every parked run, and the existing index leads with
-- the company: reaching the approvers of one area means scanning every grant
-- that area has, of every role.
create index if not exists role_grants_role_scope_idx
    on role_grants (role, company_id, area_id);
