package ticket

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func (p *Postgres) RecordInspection(
	ctx context.Context, in InspectionInput,
) (Ticket, bool, error) {
	if err := validateInspection(in); err != nil {
		return Ticket{}, false, err
	}
	tx, current, err := p.lockedTicket(ctx, in.Ref.Key)
	if err != nil {
		return Ticket{}, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := requireCurrent(current, in.Ref); err != nil {
		return Ticket{}, false, err
	}
	if current.Current.Phase != PhaseCollecting {
		return Ticket{}, false, phaseError(current.Current.Phase)
	}
	if current.Current.Snapshot == in.Snapshot {
		return current, false, nil
	}
	if err := setInspection(ctx, tx, in); err != nil {
		return Ticket{}, false, err
	}
	return commitTicket(ctx, tx, in.Ref.Key, true)
}

func (p *Postgres) AwaitApproval(
	ctx context.Context, in ApprovalInput,
) (Ticket, bool, error) {
	if err := validateApproval(in); err != nil {
		return Ticket{}, false, err
	}
	tx, ticket, err := p.lockedTicket(ctx, in.Ref.Key)
	if err != nil {
		return Ticket{}, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := requireCurrent(ticket, in.Ref); err != nil {
		return Ticket{}, false, err
	}
	approval := Approval{RunID: in.RunID, AtSeq: in.AtSeq, Snapshot: in.Snapshot}
	if ticket.Current.Phase == PhaseAwaitingApproval && ticket.Current.Approval != nil &&
		*ticket.Current.Approval == approval {
		return ticket, false, nil
	}
	if ticket.Current.Phase != PhaseCollecting {
		return Ticket{}, false, phaseError(ticket.Current.Phase)
	}
	if ticket.Current.Snapshot != in.Snapshot {
		return Ticket{}, false, ErrSnapshotMoved
	}
	if err := setApproval(ctx, tx, in); err != nil {
		return Ticket{}, false, err
	}
	return commitTicket(ctx, tx, in.Ref.Key, true)
}

func (p *Postgres) ClaimExecution(
	ctx context.Context, in ClaimInput,
) (Ticket, bool, error) {
	if err := validateClaim(in); err != nil {
		return Ticket{}, false, err
	}
	tx, ticket, err := p.lockedTicket(ctx, in.Ref.Key)
	if err != nil {
		return Ticket{}, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if ticket.Active != nil {
		return existingClaim(ticket, in)
	}
	if err := requireCurrent(ticket, in.Ref); err != nil {
		return Ticket{}, false, err
	}
	approval := ticket.Current.Approval
	if ticket.Current.Phase != PhaseAwaitingApproval || approval == nil ||
		approval.RunID != in.RunID || approval.AtSeq != in.ApprovalAtSeq {
		return Ticket{}, false, phaseError(ticket.Current.Phase)
	}
	if err := claimRevision(ctx, tx, in); err != nil {
		return Ticket{}, false, err
	}
	return commitTicket(ctx, tx, in.Ref.Key, true)
}

func (p *Postgres) ClaimExecutionWithAttempt(
	ctx context.Context, in ClaimAttemptInput,
) (Ticket, ExternalAttempt, bool, error) {
	if err := validateClaimAttempt(in); err != nil {
		return Ticket{}, ExternalAttempt{}, false, err
	}
	tx, current, err := p.lockedTicket(ctx, in.Claim.Ref.Key)
	if err != nil {
		return Ticket{}, ExternalAttempt{}, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if current.Active != nil {
		if _, _, err := existingClaim(current, in.Claim); err != nil {
			return Ticket{}, ExternalAttempt{}, false, err
		}
		stored, err := readExternalAttempt(ctx, tx, in.Attempt.IdemKey)
		want := attemptFor(*current.Active, in)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return Ticket{}, ExternalAttempt{}, false, err
		}
		if err != nil || !sameExternalAttempt(stored, want) {
			return Ticket{}, ExternalAttempt{}, false, ErrAttemptConflict
		}
		return current, stored, false, nil
	}
	if err := requireCurrent(current, in.Claim.Ref); err != nil {
		return Ticket{}, ExternalAttempt{}, false, err
	}
	approval := current.Current.Approval
	if current.Current.Phase != PhaseAwaitingApproval || approval == nil ||
		approval.RunID != in.Claim.RunID || approval.AtSeq != in.Claim.ApprovalAtSeq {
		return Ticket{}, ExternalAttempt{}, false, phaseError(current.Current.Phase)
	}
	execution := Execution{
		Ref: in.Claim.Ref, RunID: in.Claim.RunID,
		ApprovalAtSeq: in.Claim.ApprovalAtSeq, Snapshot: approval.Snapshot,
	}
	attempt := attemptFor(execution, in)
	if err := claimRevision(ctx, tx, in.Claim); err != nil {
		return Ticket{}, ExternalAttempt{}, false, err
	}
	if err := insertExternalAttempt(ctx, tx, attempt); err != nil {
		return Ticket{}, ExternalAttempt{}, false, err
	}
	ticket, changed, err := commitTicket(ctx, tx, in.Claim.Ref.Key, true)
	return ticket, attempt, changed, err
}

func (p *Postgres) Close(ctx context.Context, in CloseInput) (Ticket, bool, error) {
	if err := validateClose(in); err != nil {
		return Ticket{}, false, err
	}
	tx, ticket, err := p.lockedTicket(ctx, in.Ref.Key)
	if err != nil {
		return Ticket{}, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := requireCurrent(ticket, in.Ref); err != nil {
		return Ticket{}, false, err
	}
	if sameOutcome(ticket.Current.Outcome, in.Phase, in.Result) {
		return ticket, false, nil
	}
	if ticket.Active != nil && ticket.Active.Ref == in.Ref {
		return Ticket{}, false, ErrExecutionActive
	}
	if ticket.Current.Phase.terminal() {
		return Ticket{}, false, ErrTerminal
	}
	if err := closeRevision(ctx, tx, in); err != nil {
		return Ticket{}, false, err
	}
	return commitTicket(ctx, tx, in.Ref.Key, true)
}

func (p *Postgres) FinishExecution(ctx context.Context, in FinishInput) (Ticket, error) {
	if err := validateFinish(in); err != nil {
		return Ticket{}, err
	}
	tx, ticket, err := p.lockedTicket(ctx, in.Execution.Ref.Key)
	if err != nil {
		return Ticket{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if ticket.Active == nil {
		return finishedBefore(ctx, tx, ticket, in)
	}
	if *ticket.Active != in.Execution {
		return Ticket{}, ErrExecutionActive
	}
	if err := finishRevision(ctx, tx, in); err != nil {
		return Ticket{}, err
	}
	ticket, _, err = commitTicket(ctx, tx, in.Execution.Ref.Key, false)
	return ticket, err
}

func setApproval(ctx context.Context, tx pgx.Tx, in ApprovalInput) error {
	_, err := tx.Exec(ctx, `
		update governed_ticket_revisions
		set phase = $3, approval_run_id = $4, approval_at_seq = $5, updated_at = $6
		where ticket_key = $1 and revision = $2`,
		string(in.Ref.Key), in.Ref.Revision, string(PhaseAwaitingApproval),
		string(in.RunID), in.AtSeq, in.At.UTC())
	if err != nil {
		return fmt.Errorf("ticket: await approval: %w", err)
	}
	return touchTicket(ctx, tx, in.Ref.Key, in.At)
}

func setInspection(ctx context.Context, tx pgx.Tx, in InspectionInput) error {
	_, err := tx.Exec(ctx, `
		update governed_ticket_revisions
		set snapshot_ref = $3, snapshot_digest = $4, updated_at = $5
		where ticket_key = $1 and revision = $2`,
		string(in.Ref.Key), in.Ref.Revision, in.Snapshot.Ref, in.Snapshot.Digest, in.At.UTC())
	if err != nil {
		return fmt.Errorf("ticket: record inspection: %w", err)
	}
	return touchTicket(ctx, tx, in.Ref.Key, in.At)
}

func claimRevision(ctx context.Context, tx pgx.Tx, in ClaimInput) error {
	if _, err := tx.Exec(ctx, `
		update governed_ticket_revisions set phase = $3, updated_at = $4
		where ticket_key = $1 and revision = $2`, string(in.Ref.Key), in.Ref.Revision,
		string(PhaseExecuting), in.At.UTC()); err != nil {
		return fmt.Errorf("ticket: claim revision: %w", err)
	}
	_, err := tx.Exec(ctx, `
		update governed_tickets set active_revision = $2, updated_at = $3
		where ticket_key = $1`, string(in.Ref.Key), in.Ref.Revision, in.At.UTC())
	if err != nil {
		return fmt.Errorf("ticket: claim ticket: %w", err)
	}
	return nil
}

func closeRevision(ctx context.Context, tx pgx.Tx, in CloseInput) error {
	_, err := tx.Exec(ctx, `
		update governed_ticket_revisions
		set phase = $3, outcome_ref = $4, outcome_digest = $5, updated_at = $6
		where ticket_key = $1 and revision = $2`, string(in.Ref.Key), in.Ref.Revision,
		string(in.Phase), in.Result.Ref, in.Result.Digest, in.At.UTC())
	if err != nil {
		return fmt.Errorf("ticket: close revision: %w", err)
	}
	return touchTicket(ctx, tx, in.Ref.Key, in.At)
}

func finishRevision(ctx context.Context, tx pgx.Tx, in FinishInput) error {
	if _, err := tx.Exec(ctx, `
		update governed_ticket_revisions
		set phase = $3, outcome_ref = $4, outcome_digest = $5, updated_at = $6
		where ticket_key = $1 and revision = $2`,
		string(in.Execution.Ref.Key), in.Execution.Ref.Revision, string(in.Phase),
		in.Result.Ref, in.Result.Digest, in.At.UTC()); err != nil {
		return fmt.Errorf("ticket: finish revision: %w", err)
	}
	_, err := tx.Exec(ctx, `
		update governed_tickets set active_revision = null, updated_at = $2
		where ticket_key = $1`, string(in.Execution.Ref.Key), in.At.UTC())
	if err != nil {
		return fmt.Errorf("ticket: finish ticket: %w", err)
	}
	return nil
}

func finishedBefore(
	ctx context.Context, tx pgx.Tx, ticket Ticket, in FinishInput,
) (Ticket, error) {
	revision, err := readRevision(ctx, tx, in.Execution.Ref)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Ticket{}, ErrMoved
		}
		return Ticket{}, err
	}
	if sameOutcome(revision.Outcome, in.Phase, in.Result) {
		return ticket, nil
	}
	return Ticket{}, ErrMoved
}
