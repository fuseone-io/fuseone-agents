package ticket

import (
	"context"
	"fmt"
)

// MarkExecutionNeedsAttention publishes an operator-visible result without
// releasing the execution claim. No newer revision may start another external
// write while the original attempt is still being observed.
func (p *Postgres) MarkExecutionNeedsAttention(
	ctx context.Context, in AttentionInput,
) (Ticket, error) {
	if err := validateAttention(in); err != nil {
		return Ticket{}, err
	}
	tx, held, err := p.lockedTicket(ctx, in.Execution.Ref.Key)
	if err != nil {
		return Ticket{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if held.Active == nil || *held.Active != in.Execution {
		return Ticket{}, ErrExecutionActive
	}
	revision, err := readRevision(ctx, tx, in.Execution.Ref)
	if err != nil {
		return Ticket{}, err
	}
	if sameOutcome(revision.Outcome, PhaseNeedsAttention, in.Result) {
		return held, nil
	}
	if _, err := tx.Exec(ctx, `
		update governed_ticket_revisions
		set phase = $3, outcome_ref = $4, outcome_digest = $5,
		    outcome_announced_at = null, outcome_claimed_by = '', outcome_claim_until = null,
		    updated_at = $6
		where ticket_key = $1 and revision = $2`,
		string(in.Execution.Ref.Key), in.Execution.Ref.Revision,
		string(PhaseNeedsAttention), in.Result.Ref, in.Result.Digest, in.At.UTC()); err != nil {
		return Ticket{}, fmt.Errorf("ticket: mark execution for attention: %w", err)
	}
	if err := touchTicket(ctx, tx, in.Execution.Ref.Key, in.At); err != nil {
		return Ticket{}, err
	}
	current, _, err := commitTicket(ctx, tx, in.Execution.Ref.Key, true)
	return current, err
}
