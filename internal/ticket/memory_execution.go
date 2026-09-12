package ticket

import (
	"context"
	"fmt"

	"github.com/fuseone/agents/internal/domain"
)

func (m *Memory) RecordInspection(
	ctx context.Context, in InspectionInput,
) (Ticket, bool, error) {
	if err := validateInspection(in); err != nil {
		return Ticket{}, false, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	ticket, err := m.currentFor(ctx, in.Ref)
	if err != nil {
		return Ticket{}, false, err
	}
	if ticket.Current.Phase != PhaseCollecting {
		return Ticket{}, false, phaseError(ticket.Current.Phase)
	}
	if ticket.Current.Snapshot == in.Snapshot {
		return cloneTicket(ticket), false, nil
	}
	ticket.Current.Snapshot = in.Snapshot
	ticket.Current.UpdatedAt, ticket.UpdatedAt = in.At.UTC(), in.At.UTC()
	m.putCurrent(ticket)
	return cloneTicket(ticket), true, nil
}

func (m *Memory) AwaitApproval(ctx context.Context, in ApprovalInput) (Ticket, bool, error) {
	if err := validateApproval(in); err != nil {
		return Ticket{}, false, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	ticket, err := m.currentFor(ctx, in.Ref)
	if err != nil {
		return Ticket{}, false, err
	}
	approval := Approval{RunID: in.RunID, AtSeq: in.AtSeq, Snapshot: in.Snapshot}
	if ticket.Current.Phase == PhaseAwaitingApproval && ticket.Current.Approval != nil &&
		*ticket.Current.Approval == approval {
		return cloneTicket(ticket), false, nil
	}
	if ticket.Current.Phase != PhaseCollecting {
		return Ticket{}, false, phaseError(ticket.Current.Phase)
	}
	if ticket.Current.Snapshot != in.Snapshot {
		return Ticket{}, false, ErrSnapshotMoved
	}
	ticket.Current.Phase = PhaseAwaitingApproval
	ticket.Current.Approval = &approval
	ticket.Current.UpdatedAt, ticket.UpdatedAt = in.At.UTC(), in.At.UTC()
	m.putCurrent(ticket)
	return cloneTicket(ticket), true, nil
}

func (m *Memory) ClaimExecution(ctx context.Context, in ClaimInput) (Ticket, bool, error) {
	if err := validateClaim(in); err != nil {
		return Ticket{}, false, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	ticket, err := m.ticketFor(ctx, in.Ref.Key)
	if err != nil {
		return Ticket{}, false, err
	}
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
	execution := Execution{
		Ref: in.Ref, RunID: in.RunID, ApprovalAtSeq: in.ApprovalAtSeq,
		Snapshot: approval.Snapshot,
	}
	ticket.Active = &execution
	ticket.Current.Phase, ticket.Current.UpdatedAt = PhaseExecuting, in.At.UTC()
	ticket.UpdatedAt = in.At.UTC()
	m.putCurrent(ticket)
	return cloneTicket(ticket), true, nil
}

func (m *Memory) ClaimExecutionWithAttempt(
	ctx context.Context, in ClaimAttemptInput,
) (Ticket, ExternalAttempt, bool, error) {
	if err := validateClaimAttempt(in); err != nil {
		return Ticket{}, ExternalAttempt{}, false, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	ticket, err := m.ticketFor(ctx, in.Claim.Ref.Key)
	if err != nil {
		return Ticket{}, ExternalAttempt{}, false, err
	}
	if ticket.Active != nil {
		if _, _, err := existingClaim(ticket, in.Claim); err != nil {
			return Ticket{}, ExternalAttempt{}, false, err
		}
		stored, ok := m.attempts[in.Attempt.IdemKey]
		want := attemptFor(*ticket.Active, in)
		if !ok || !sameExternalAttempt(stored, want) {
			return Ticket{}, ExternalAttempt{}, false, ErrAttemptConflict
		}
		return cloneTicket(ticket), stored, false, nil
	}
	if _, exists := m.attempts[in.Attempt.IdemKey]; exists {
		return Ticket{}, ExternalAttempt{}, false, ErrAttemptConflict
	}
	if err := requireCurrent(ticket, in.Claim.Ref); err != nil {
		return Ticket{}, ExternalAttempt{}, false, err
	}
	approval := ticket.Current.Approval
	if ticket.Current.Phase != PhaseAwaitingApproval || approval == nil ||
		approval.RunID != in.Claim.RunID || approval.AtSeq != in.Claim.ApprovalAtSeq {
		return Ticket{}, ExternalAttempt{}, false, phaseError(ticket.Current.Phase)
	}
	execution := Execution{
		Ref: in.Claim.Ref, RunID: in.Claim.RunID,
		ApprovalAtSeq: in.Claim.ApprovalAtSeq, Snapshot: approval.Snapshot,
	}
	attempt := attemptFor(execution, in)
	for _, stored := range m.attempts {
		if stored.Execution.Ref == execution.Ref {
			return Ticket{}, ExternalAttempt{}, false, ErrAttemptConflict
		}
	}
	ticket.Active = &execution
	ticket.Current.Phase, ticket.Current.UpdatedAt = PhaseExecuting, in.Claim.At.UTC()
	ticket.UpdatedAt = in.Claim.At.UTC()
	m.attempts[attempt.IdemKey] = attempt
	m.putCurrent(ticket)
	return cloneTicket(ticket), attempt, true, nil
}

func (m *Memory) Close(ctx context.Context, in CloseInput) (Ticket, bool, error) {
	if err := validateClose(in); err != nil {
		return Ticket{}, false, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	ticket, err := m.currentFor(ctx, in.Ref)
	if err != nil {
		return Ticket{}, false, err
	}
	if sameOutcome(ticket.Current.Outcome, in.Phase, in.Result) {
		return cloneTicket(ticket), false, nil
	}
	if ticket.Active != nil && ticket.Active.Ref == in.Ref {
		return Ticket{}, false, ErrExecutionActive
	}
	if ticket.Current.Phase.terminal() {
		return Ticket{}, false, ErrTerminal
	}
	ticket.Current.Phase = in.Phase
	ticket.Current.Outcome = &Outcome{Phase: in.Phase, Result: in.Result}
	ticket.Current.UpdatedAt, ticket.UpdatedAt = in.At.UTC(), in.At.UTC()
	m.putCurrent(ticket)
	return cloneTicket(ticket), true, nil
}

func (m *Memory) FinishExecution(ctx context.Context, in FinishInput) (Ticket, error) {
	if err := validateFinish(in); err != nil {
		return Ticket{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	ticket, err := m.ticketFor(ctx, in.Execution.Ref.Key)
	if err != nil {
		return Ticket{}, err
	}
	if ticket.Active == nil {
		revision, exists := m.revisions[in.Execution.Ref.Key][in.Execution.Ref.Revision]
		if exists && sameOutcome(revision.Outcome, in.Phase, in.Result) {
			return cloneTicket(ticket), nil
		}
		return Ticket{}, ErrMoved
	}
	if *ticket.Active != in.Execution {
		return Ticket{}, ErrExecutionActive
	}
	revision := m.revisions[in.Execution.Ref.Key][in.Execution.Ref.Revision]
	revision.Phase = in.Phase
	revision.Outcome = &Outcome{Phase: in.Phase, Result: in.Result}
	revision.UpdatedAt = in.At.UTC()
	m.revisions[in.Execution.Ref.Key][in.Execution.Ref.Revision] = revision
	ticket.Active, ticket.UpdatedAt = nil, in.At.UTC()
	if ticket.Current.Ref == in.Execution.Ref {
		ticket.Current = revision
	}
	m.tickets[in.Execution.Ref.Key] = ticket
	return cloneTicket(ticket), nil
}

func (m *Memory) ticketFor(ctx context.Context, key domain.TicketKey) (Ticket, error) {
	if err := ctx.Err(); err != nil {
		return Ticket{}, err
	}
	ticket, ok := m.tickets[key]
	if !ok {
		return Ticket{}, ErrNotFound
	}
	return ticket, nil
}

func (m *Memory) currentFor(ctx context.Context, ref domain.TicketRef) (Ticket, error) {
	ticket, err := m.ticketFor(ctx, ref.Key)
	if err != nil {
		return Ticket{}, err
	}
	return ticket, requireCurrent(ticket, ref)
}

func (m *Memory) putCurrent(ticket Ticket) {
	m.tickets[ticket.Key] = ticket
	m.revisions[ticket.Key][ticket.Current.Ref.Revision] = ticket.Current
}

func existingClaim(ticket Ticket, in ClaimInput) (Ticket, bool, error) {
	if ticket.Active.Ref == in.Ref && ticket.Active.RunID == in.RunID &&
		ticket.Active.ApprovalAtSeq == in.ApprovalAtSeq {
		return cloneTicket(ticket), false, nil
	}
	return Ticket{}, false, ErrExecutionActive
}

func phaseError(phase Phase) error {
	if phase.terminal() {
		return fmt.Errorf("%w: %s", ErrTerminal, phase)
	}
	return fmt.Errorf("%w: %s", ErrPhase, phase)
}

func sameOutcome(outcome *Outcome, phase Phase, result ContentRef) bool {
	return outcome != nil && outcome.Phase == phase && outcome.Result == result
}
