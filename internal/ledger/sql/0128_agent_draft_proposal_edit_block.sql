-- 0128_agent_draft_proposal_edit_block.sql
-- A proposal may edit one excerpt of a block: a change to a long block names
-- only what it changes, since rewriting the block whole does not fit a turn.
-- Widening the check only: every row the previous binary wrote still passes,
-- and the previous binary never writes the new kind.

alter table agent_draft_proposals
    drop constraint if exists agent_draft_proposals_kind_check;

alter table agent_draft_proposals
    add constraint agent_draft_proposals_kind_check check (kind in (
        'set_identity', 'set_standing_approvals', 'set_model', 'set_fallback',
        'set_cache_prefix_ttl', 'set_reply_locale', 'add_tool', 'remove_tool',
        'set_budget', 'set_triggers', 'set_memory_learning', 'set_approval_delivery',
        'set_emits', 'add_step', 'edit_step', 'remove_step',
        'insert_block', 'replace_block', 'edit_block', 'remove_block', 'move_block'));
