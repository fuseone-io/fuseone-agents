-- What a workbench turn's answer carried that never became a card (#413): operations
-- discarded on the way in (unreadable, past the limit per answer, or of a thread erased
-- during the call), and those made against a draft that moved during the call. Kept on the
-- turn so a retry of its key and a reload say it too, not only the first answer.
--
-- Additive. A replica from before this migration settles a turn without writing them,
-- which leaves zero: no worse than before, when nothing was kept.
alter table agent_draft_turns
    add column dropped integer not null default 0 check (dropped >= 0),
    add column stale   integer not null default 0 check (stale >= 0);
