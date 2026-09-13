alter table governed_ticket_revisions
    drop constraint governed_ticket_revisions_check1;

alter table governed_ticket_revisions
    add constraint governed_ticket_revision_snapshot_shape check (
        (snapshot_ref = '' and snapshot_digest = '') or
        (snapshot_ref <> '' and snapshot_digest <> '')
    ),
    add constraint governed_ticket_revision_approval_shape check (
        (approval_run_id = '' and approval_at_seq = 0) or
        (approval_run_id <> '' and approval_at_seq > 0 and
         snapshot_ref <> '' and snapshot_digest <> '')
    );
