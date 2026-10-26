-- The subset of cache writes made with the hour-long window, which bills at
-- its own rate. A dedicated counter beside the existing four: the total stays
-- in cache_write_tokens, so every reader of the total stays correct.
alter table run_steps
    add column cache_write_1h_tokens bigint not null default 0;
