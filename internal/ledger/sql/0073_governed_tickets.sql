create table governed_tickets (
    ticket_key       text        primary key,
    company_id       text        not null,
    area_id          text        not null,
    requested_by     text        not null,
    addressed_by     text        not null default '',
    current_revision bigint      not null check (current_revision > 0),
    active_revision  bigint,
    created_at       timestamptz not null,
    updated_at       timestamptz not null,
    check (company_id <> '' and area_id <> '' and requested_by <> ''),
    check (active_revision is null or active_revision > 0)
);

create table governed_ticket_revisions (
    ticket_key       text        not null references governed_tickets(ticket_key) on delete cascade,
    revision         bigint      not null check (revision > 0),
    phase            text        not null check (phase in (
        'collecting', 'awaiting_approval', 'executing',
        'completed', 'rejected', 'cancelled'
    )),
    draft_ref        text        not null,
    draft_digest     text        not null,
    approval_run_id  text        not null default '',
    approval_at_seq  bigint      not null default 0,
    snapshot_ref     text        not null default '',
    snapshot_digest  text        not null default '',
    recipients       text[]      not null default '{}',
    outcome_ref      text        not null default '',
    outcome_digest   text        not null default '',
    created_at       timestamptz not null,
    updated_at       timestamptz not null,
    primary key (ticket_key, revision),
    check (draft_ref <> '' and draft_digest <> ''),
    check (
        (approval_run_id = '' and approval_at_seq = 0 and snapshot_ref = '' and snapshot_digest = '') or
        (approval_run_id <> '' and approval_at_seq > 0 and snapshot_ref <> '' and snapshot_digest <> '')
    ),
    check ((outcome_ref = '' and outcome_digest = '') or
           (outcome_ref <> '' and outcome_digest <> '')),
    check (cardinality(recipients) <= 20 and array_position(recipients, '') is null)
);

alter table governed_tickets
    add constraint governed_tickets_active_revision_fk
    foreign key (ticket_key, active_revision)
    references governed_ticket_revisions(ticket_key, revision)
    deferrable initially deferred;

create table governed_ticket_events (
    event_id   text        primary key,
    ticket_key text        not null references governed_tickets(ticket_key) on delete cascade,
    revision   bigint      not null,
    occurred_at timestamptz not null,
    foreign key (ticket_key, revision)
        references governed_ticket_revisions(ticket_key, revision)
);

create index governed_ticket_events_ticket_idx
    on governed_ticket_events (ticket_key, revision);
