alter table governed_tickets
	add column agent_id text not null default '',
	add column run_as text not null default '',
    add column origin_connection text not null default '',
    add column origin_conversation text not null default '',
    add column origin_root text not null default '';

create unique index governed_tickets_origin_idx
    on governed_tickets (origin_connection, origin_conversation, origin_root)
    where origin_connection <> '' and origin_conversation <> '' and origin_root <> '';

create index governed_tickets_scope_idx
	on governed_tickets (company_id, area_id);

-- A ticket arrival is classified before Slack is acknowledged and consumed
-- afterwards. Keeping the classification beside the inbox row makes that
-- decision survive a process death without asking a possibly changed rule.
alter table channel_inbox
    add column ticket_intent jsonb;

-- Ticket root matching looks up exactly one configured conversation. The
-- general settings index begins with scope, which is not known at this door.
create index settings_conversation_name_idx
    on settings (name)
    where kind = 'conversation' and enabled;

-- A terminal external effect still owes the root thread one deterministic
-- answer. Leasing the obligation makes worker death retryable; marking before
-- posting would make a lost process indistinguishable from a delivered reply.
alter table governed_ticket_revisions
    add column outcome_claimed_by text not null default '',
    add column outcome_claim_until timestamptz,
    add column outcome_announced_at timestamptz,
    add column approval_superseded_at timestamptz;

alter table governed_ticket_revisions
    drop constraint governed_ticket_revisions_phase_check,
    add constraint governed_ticket_revisions_phase_check check (phase in (
        'collecting', 'awaiting_approval', 'executing', 'needs_attention',
        'completed', 'rejected', 'cancelled'
    ));

create index governed_ticket_outcomes_due_idx
    on governed_ticket_revisions (updated_at, ticket_key, revision)
    where phase in ('completed', 'rejected', 'needs_attention') and outcome_ref <> ''
      and outcome_announced_at is null;

-- The announcement projection needs the immutable ticket revision without
-- replaying run_started for every parked run in every sweep.
alter table runs
    add column ticket_key text not null default '',
    add column ticket_revision bigint not null default 0;

-- A result that cannot yet be proved final stays visible to the operator and
-- is observed again with GET. Rows written before this release used manual as
-- a terminal journal state and have neither result nor next check; keep those
-- historical rows valid without putting an incomplete attempt back to work.
alter table governed_external_attempts
    drop constraint governed_external_attempts_result_shape_v1,
    add constraint governed_external_attempts_result_shape check (
        (status in ('prepared', 'pending') and result_ref = '' and result_digest = '' and
         outcome_code = '' and next_check_at is not null and not settled) or
        (status in ('confirmed', 'terminal') and result_ref <> '' and
         result_digest <> '' and outcome_code <> '' and
         ((not settled and next_check_at is not null) or
          (settled and next_check_at is null))) or
        (status = 'manual' and outcome_code <> '' and not settled and
         ((result_ref = '' and result_digest = '' and next_check_at is null) or
          (result_ref <> '' and result_digest <> '' and next_check_at is not null)))
    );

drop index governed_external_attempts_due_idx;
create index governed_external_attempts_due_idx
    on governed_external_attempts (next_check_at, created_at)
    where not settled and next_check_at is not null;
