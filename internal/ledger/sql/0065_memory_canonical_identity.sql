-- The identity a duplicate check can trust.
--
-- assertion_id hashes the strings as typed, which is right for naming a row:
-- what a memory is called must not change under it. But it makes "Slack Alerts"
-- and " slack   alerts " two memories that never find each other, so the
-- question "does this already exist" cannot be asked of it.
--
-- Nullable, and deliberately not unique. Nullable because rows written before
-- this migration have no key and are filled as they are read, so an upgrade
-- does not have to finish before the platform serves. Not unique because the
-- duplicates this key reveals are already in the table: a constraint would
-- refuse the upgrade rather than surface them.
alter table memory_assertions
	add column if not exists canonical_identity_key text;

alter table memory_suggestions
	add column if not exists canonical_identity_key text;

create index if not exists memory_assertions_canonical_idx
	on memory_assertions (company_id, area_id, agent_id, canonical_identity_key);

create index if not exists memory_suggestions_canonical_idx
	on memory_suggestions (company_id, area_id, agent_id, canonical_identity_key);

-- The rows still waiting to be filled. A partial index costs nothing once the
-- last legacy row is hydrated, because it indexes nothing: it shrinks to empty
-- on its own rather than becoming a thing somebody has to remember to drop.
create index if not exists memory_assertions_unkeyed_idx
	on memory_assertions (company_id, area_id, agent_id)
	where canonical_identity_key is null;

create index if not exists memory_suggestions_unkeyed_idx
	on memory_suggestions (company_id, area_id, agent_id)
	where canonical_identity_key is null;
