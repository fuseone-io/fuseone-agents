package ticket

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/fuseone/agents/internal/domain"
)

type rowQuery interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

const ticketColumns = `
	t.ticket_key, t.company_id, t.area_id, t.requested_by, t.addressed_by,
	t.current_revision, t.active_revision, t.created_at, t.updated_at,
	c.phase, c.draft_ref, c.draft_digest,
	c.approval_run_id, c.approval_at_seq, c.snapshot_ref, c.snapshot_digest,
	c.recipients, c.outcome_ref, c.outcome_digest, c.created_at, c.updated_at,
	a.approval_run_id, a.approval_at_seq, a.snapshot_ref, a.snapshot_digest`

func readTicket(
	ctx context.Context, db rowQuery, key domain.TicketKey, locked bool,
) (Ticket, error) {
	query := `select ` + ticketColumns + `
		from governed_tickets t
		join governed_ticket_revisions c
		  on c.ticket_key = t.ticket_key and c.revision = t.current_revision
		left join governed_ticket_revisions a
		  on a.ticket_key = t.ticket_key and a.revision = t.active_revision
		where t.ticket_key = $1`
	if locked {
		query += ` for update of t`
	}
	var record ticketRecord
	err := record.scan(db.QueryRow(ctx, query, string(key)))
	if errors.Is(err, pgx.ErrNoRows) {
		return Ticket{}, ErrNotFound
	}
	if err != nil {
		return Ticket{}, fmt.Errorf("ticket: read current: %w", err)
	}
	return record.value(), nil
}

type ticketRecord struct {
	ticket                                             Ticket
	key, company, area, requester, phase               string
	currentRevision                                    int64
	activeRevision                                     *int64
	approvalRun, snapshotRef, snapshotDigest           string
	approvalSeq                                        int64
	recipients                                         []string
	outcomeRef, outcomeDigest                          string
	activeRun, activeSnapshotRef, activeSnapshotDigest *string
	activeApprovalSeq                                  *int64
}

func (r *ticketRecord) scan(row pgx.Row) error {
	return row.Scan(
		&r.key, &r.company, &r.area, &r.requester, &r.ticket.AddressedBy,
		&r.currentRevision, &r.activeRevision, &r.ticket.CreatedAt, &r.ticket.UpdatedAt,
		&r.phase, &r.ticket.Current.Draft.Ref, &r.ticket.Current.Draft.Digest,
		&r.approvalRun, &r.approvalSeq, &r.snapshotRef, &r.snapshotDigest,
		&r.recipients, &r.outcomeRef, &r.outcomeDigest,
		&r.ticket.Current.CreatedAt, &r.ticket.Current.UpdatedAt,
		&r.activeRun, &r.activeApprovalSeq, &r.activeSnapshotRef, &r.activeSnapshotDigest,
	)
}

func (r ticketRecord) value() Ticket {
	ticket := r.ticket
	ticket.Key = domain.TicketKey(r.key)
	ticket.Scope = domain.Scope{Company: domain.CompanyID(r.company), Area: domain.AreaID(r.area)}
	ticket.RequestedBy = domain.UserID(r.requester)
	ticket.Current.Ref = domain.TicketRef{Key: ticket.Key, Revision: r.currentRevision}
	ticket.Current.Phase, ticket.Current.Recipients = Phase(r.phase), recipientIDs(r.recipients)
	fillRevision(&ticket.Current, r.approvalRun, r.approvalSeq, r.snapshotRef, r.snapshotDigest,
		r.outcomeRef, r.outcomeDigest)
	if r.activeRevision != nil {
		ticket.Active = &Execution{
			Ref:   domain.TicketRef{Key: ticket.Key, Revision: *r.activeRevision},
			RunID: domain.RunID(value(r.activeRun)), ApprovalAtSeq: value(r.activeApprovalSeq),
			Snapshot: ContentRef{Ref: value(r.activeSnapshotRef), Digest: value(r.activeSnapshotDigest)},
		}
	}
	return ticket
}

func readRevision(ctx context.Context, db rowQuery, ref domain.TicketRef) (Revision, error) {
	var revision Revision
	var phase, approvalRun, snapshotRef, snapshotDigest, outcomeRef, outcomeDigest string
	var approvalSeq int64
	var recipients []string
	err := db.QueryRow(ctx, `
		select phase, draft_ref, draft_digest,
		       approval_run_id, approval_at_seq, snapshot_ref, snapshot_digest,
		       recipients, outcome_ref, outcome_digest, created_at, updated_at
		from governed_ticket_revisions
		where ticket_key = $1 and revision = $2`, string(ref.Key), ref.Revision).Scan(
		&phase, &revision.Draft.Ref, &revision.Draft.Digest,
		&approvalRun, &approvalSeq, &snapshotRef, &snapshotDigest,
		&recipients, &outcomeRef, &outcomeDigest,
		&revision.CreatedAt, &revision.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Revision{}, ErrNotFound
	}
	if err != nil {
		return Revision{}, fmt.Errorf("ticket: read revision: %w", err)
	}
	revision.Ref, revision.Phase = ref, Phase(phase)
	revision.Recipients = recipientIDs(recipients)
	fillRevision(&revision, approvalRun, approvalSeq, snapshotRef, snapshotDigest,
		outcomeRef, outcomeDigest)
	return revision, nil
}

func recipientIDs(recipients []string) []domain.UserID {
	out := make([]domain.UserID, len(recipients))
	for i, recipient := range recipients {
		out[i] = domain.UserID(recipient)
	}
	return out
}

func fillRevision(
	revision *Revision, approvalRun string, approvalSeq int64,
	snapshotRef, snapshotDigest, outcomeRef, outcomeDigest string,
) {
	if approvalRun != "" {
		revision.Approval = &Approval{
			RunID: domain.RunID(approvalRun), AtSeq: approvalSeq,
			Snapshot: ContentRef{Ref: snapshotRef, Digest: snapshotDigest},
		}
	}
	if outcomeRef != "" {
		revision.Outcome = &Outcome{
			Phase: revision.Phase, Result: ContentRef{Ref: outcomeRef, Digest: outcomeDigest},
		}
	}
}

func value[T any](pointer *T) (zero T) {
	if pointer != nil {
		return *pointer
	}
	return zero
}
