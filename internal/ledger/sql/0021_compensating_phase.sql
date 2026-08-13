-- A run somebody abandoned waits for a worker to carry out its undos, so the
-- claim index has to reach it. The index is partial to stay small however many
-- finished runs pile up, which means a new claimable phase is a new index.
drop index if exists runs_resumable_idx;

create index runs_resumable_idx on runs (updated_at)
    where phase in ('running', 'awaiting_tool', 'parked', 'compensating');
