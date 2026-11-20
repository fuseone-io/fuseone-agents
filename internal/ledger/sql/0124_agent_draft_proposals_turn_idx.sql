-- fuseone:no-transaction
--
-- The proposals of a workbench turn, found by the turn (#413): a turn's prompt reads what
-- became of the proposals of the turns in its window, and agent_draft_proposals is
-- append-only, so without this every turn would scan the whole table to answer a handful.
--
-- Built concurrently, outside a transaction (the marker on the first line): the previous
-- release keeps appending proposals while this runs, and a plain create index would hold
-- every one of those writes for the length of the build. Partial, because a proposal made
-- outside a conversation has no turn.

create index concurrently if not exists agent_draft_proposals_turn_idx
    on agent_draft_proposals (turn_id, seq) where turn_id is not null;
