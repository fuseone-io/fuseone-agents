-- 0046_admin_role.sql
-- Admin is the operational role for the person responsible for the
-- installation. It is still scoped: the power comes from pairing it with the
-- installation scope, not from a wildcard permission model.

alter table role_grants
    drop constraint if exists role_grants_role_check;

alter table role_grants
    add constraint role_grants_role_check
    check (role in ('admin', 'author', 'approver', 'curator', 'auditor'));

update role_grants
set area_id = ''
where company_id = '*'
  and area_id <> '';

alter table role_grants
    drop constraint if exists role_grants_installation_area_check;

alter table role_grants
    add constraint role_grants_installation_area_check
    check (company_id <> '*' or area_id = '');
