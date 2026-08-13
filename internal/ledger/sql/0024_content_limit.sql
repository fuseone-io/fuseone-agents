-- What was stored, when what arrived was bigger than the limit.
--
-- Object storage is optional and this installation has none, so bulky payloads
-- live in Postgres and there is a size past which that stops being reasonable
-- (PRD DE-03). Past it the store keeps a prefix and says so, rather than
-- writing whatever a tool returned into a row.
--
-- The digest stays the digest of the whole thing. That is what makes the
-- record honest: an auditor holding the original can still prove it is the
-- one the run used, and the trail says plainly that this copy is partial.
alter table run_content
    add column if not exists full_bytes bigint not null default 0,
    add column if not exists truncated  boolean not null default false;
