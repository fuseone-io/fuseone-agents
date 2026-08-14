-- The first administrator reaches the installation, not one company.
--
-- Their grant stopped at the company the bootstrap invented, which was right
-- until companies could be created. Now it is a deadlock: creating a company
-- needs authority above every company, granting that authority needs the same
-- authority, and an installation that already claimed its administrator has
-- nobody who can give it to anybody.
--
-- So it is widened here, and narrowly: only a grant that is company-wide, on
-- the bootstrap's own company, held by somebody who has one. That is the
-- administrator who claimed this installation and, on an installation that has
-- not grown past one company, only them.
--
-- Written as a migration rather than left to an operator because there is no
-- operator who could do it. A migration that grants authority is a thing to do
-- once, visibly, with the reason next to it.
insert into role_grants (principal_id, company_id, area_id, role)
select g.principal_id, '*', '', g.role
from role_grants g
where g.company_id = 'default'
  and g.area_id = ''
  and g.role = 'curator'
on conflict do nothing;
