-- A ticket ends when somebody says it does, not when an answer is published.
--
-- A support request is not over because it was answered once: the next thing
-- the person says is about the same request, in the same thread. So the
-- revision that was approved keeps its outcome — it is the record of what was
-- published — and the ticket goes on accepting corrections until it is closed.
--
-- Closing is therefore a fact about the ticket rather than a phase of its
-- current revision, and it is what the area's cap counts: a ticket answered
-- and never closed is still one of the open tickets that area is allowed.
alter table governed_tickets
    add column closed_at timestamptz,
    add column closed_by text not null default '',
    add constraint governed_tickets_closed_by_needs_closing
        check (closed_by = '' or closed_at is not null);

create index governed_tickets_open_idx
    on governed_tickets (company_id, area_id)
    where closed_at is null;
