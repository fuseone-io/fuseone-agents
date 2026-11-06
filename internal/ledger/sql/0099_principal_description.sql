-- What a service account is for.
--
-- A person is known by who they are. A machine is known by what it does, and a token
-- nobody can explain is a token nobody dares to revoke.
alter table principals add column if not exists description text not null default '';
