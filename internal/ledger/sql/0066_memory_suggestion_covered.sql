-- An honest ending for a suggestion nobody refused.
--
-- When shared memory already answers what a suggestion proposes, no assertion
-- is written: correcting shared memory from an agent's context would rewrite
-- what every agent reads. But the suggestion had nowhere to go. Accepting it
-- returned a conflict and left it pending, so the only exit was Dismiss —
-- recording a refusal nobody made, about a fact the platform already holds.
--
-- covered says what happened: the proposal was satisfied by memory that was
-- already there, and covered_by names it.
alter table memory_suggestions
	drop constraint if exists memory_suggestions_status_check;

alter table memory_suggestions
	add constraint memory_suggestions_status_check check (
		status in (
			'pending',
			'accepted',
			'dismissed',
			'auto_confirmed',
			'covered',
			'source_erased'
		)
	);

alter table memory_suggestions
	add column if not exists covered_by text;
