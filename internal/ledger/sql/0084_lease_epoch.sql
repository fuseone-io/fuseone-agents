-- The fencing token for worker leases. Every claim advances it; Renew and
-- Release only act while the caller's epoch is still the current one, so a
-- worker suspended past its lease wakes up fenced out instead of trampling
-- the successor that claimed the run in the meantime.
alter table runs
    add column lease_epoch bigint not null default 0;
