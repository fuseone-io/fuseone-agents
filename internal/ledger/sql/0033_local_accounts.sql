-- Accounts that do not need an identity provider.
--
-- The platform's position is that identity belongs to the customer's provider:
-- joiners and leavers are handled where they are already handled, and the
-- console never sees a password. That position is right and it left a hole —
-- until a provider is configured there is exactly one account, the one the
-- setup token created, and its session is the only way in. An installation
-- whose administrator lost that session had no door left, and no installation
-- could have two people in it before its first provider was registered.
--
-- Which matters more than convenience: the duties exist to separate an author
-- from an approver, and a platform with one account cannot demonstrate the
-- separation it is sold on.

-- The handle somebody types. Separate from `subject`, which belongs to the
-- provider that issued it, so the administrator created by the setup token can
-- be given one without rewriting where they came from.
alter table principals add column if not exists username text;
alter table principals add column if not exists password_hash text;
alter table principals add column if not exists password_set_at timestamptz;

create unique index if not exists principals_username_uniq
    on principals (lower(username)) where username is not null;

-- Failed attempts, so a console reachable from a browser is not a password
-- oracle somebody can work through at their leisure. Per principal rather than
-- per address: an address is trivially changed, and the account is the thing
-- being protected.
alter table principals add column if not exists failed_sign_ins integer not null default 0;
alter table principals add column if not exists locked_until timestamptz;
