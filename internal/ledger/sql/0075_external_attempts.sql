create table governed_external_attempts (
    idem_key          text        primary key,
    attempt_kind      text        not null,
    ticket_key        text        not null,
    revision          bigint      not null check (revision > 0),
    run_id            text        not null,
    approval_at_seq   bigint      not null check (approval_at_seq > 0),
    call_seq          bigint      not null check (call_seq > 0),
    instance_name     text        not null,
    company_id        text        not null,
    area_id           text        not null,
    contract_digest   text        not null,
    snapshot_ref      text        not null,
    snapshot_digest   text        not null,
    target_id         text        not null,
    decided_by        text        not null,
    status            text        not null default 'prepared'
        check (status in ('prepared', 'pending', 'confirmed', 'terminal', 'manual')),
    result_ref        text        not null default '',
    result_digest     text        not null default '',
    outcome_code      text        not null default '',
    checks            integer     not null default 0 check (checks >= 0),
    next_check_at     timestamptz,
    deadline_at       timestamptz not null,
    claimed_by        text        not null default '',
    claimed_until     timestamptz,
    settled           boolean     not null default false,
    created_at        timestamptz not null,
    updated_at        timestamptz not null,
    foreign key (ticket_key, revision)
        references governed_ticket_revisions(ticket_key, revision),
    unique (ticket_key, revision),
    check (attempt_kind <> '' and run_id <> '' and instance_name <> '' and
           company_id <> '' and area_id <> '' and contract_digest <> '' and
           snapshot_ref <> '' and snapshot_digest <> '' and target_id <> '' and
           decided_by <> ''),
    constraint governed_external_attempts_result_shape_v1 check (
        (status in ('prepared', 'pending') and result_ref = '' and result_digest = '' and
         outcome_code = '' and next_check_at is not null and not settled) or
        (status in ('confirmed', 'terminal') and result_ref <> '' and
         result_digest <> '' and outcome_code <> '' and
         ((not settled and next_check_at is not null) or
          (settled and next_check_at is null))) or
        (status = 'manual' and result_ref = '' and result_digest = '' and
         outcome_code <> '' and next_check_at is null and not settled)
    ),
    check ((claimed_by = '' and claimed_until is null) or
           (claimed_by <> '' and claimed_until is not null))
);

create index governed_external_attempts_due_idx
    on governed_external_attempts (next_check_at, created_at)
    where not settled and status <> 'manual';
