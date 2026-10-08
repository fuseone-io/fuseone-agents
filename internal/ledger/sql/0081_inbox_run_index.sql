-- The reporter now reads the ask's origin (channel, conversation, thread)
-- per unreported run, not just whether one exists. Without an index the
-- lateral join walks the inbox once per candidate run.
create index if not exists channel_inbox_run_idx
	on channel_inbox (run_id);
