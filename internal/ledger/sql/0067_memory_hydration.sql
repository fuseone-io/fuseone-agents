-- Repairing a projection is an act, and the trail has to hold it.
--
-- Hydration fills what the platform can now derive and could not before: which
-- step a citation names, which run produced the bytes, the whole digest, the
-- labels the run had accumulated, and the canonical identity. None of it is new
-- information — it was always in the ledger — but the projection did not carry
-- it, and without an event the log could no longer reconstruct the evidence the
-- projection now shows.
alter table memory_assertion_events
	drop constraint if exists memory_assertion_events_action_check;

alter table memory_assertion_events
	add constraint memory_assertion_events_action_check check (
		action in (
			'asserted',
			'disabled',
			'source_erased',
			'suggested',
			'accepted',
			'dismissed',
			'auto_confirmed',
			'hydrated',
			'reactivated'
		)
	);
