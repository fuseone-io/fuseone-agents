-- Each proposal's reason (NT-018 phase 3): the model's sentence on why, shown
-- beside the proposal before it is decided.
--
-- The reason is text the model wrote, so it lives in run_content under owner
-- kind 'authoring', owned by the proposal's thread: under the installation's
-- retention, and erased with the thread. The row keeps the reference only.
--
-- Only an addition. 0.118.0 does not read the column. No rollback floor.
alter table agent_draft_proposals add column reason_ref text not null default '';
