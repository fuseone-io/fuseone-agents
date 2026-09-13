package ticket

import "context"

func (m *Memory) MarkExecutionNeedsAttention(
	ctx context.Context, in AttentionInput,
) (Ticket, error) {
	if err := validateAttention(in); err != nil {
		return Ticket{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	held, err := m.ticketFor(ctx, in.Execution.Ref.Key)
	if err != nil {
		return Ticket{}, err
	}
	if held.Active == nil || *held.Active != in.Execution {
		return Ticket{}, ErrExecutionActive
	}
	revision := m.revisions[in.Execution.Ref.Key][in.Execution.Ref.Revision]
	if sameOutcome(revision.Outcome, PhaseNeedsAttention, in.Result) {
		return cloneTicket(held), nil
	}
	revision.Phase = PhaseNeedsAttention
	revision.Outcome = &Outcome{Phase: PhaseNeedsAttention, Result: in.Result}
	revision.UpdatedAt = in.At.UTC()
	m.revisions[in.Execution.Ref.Key][in.Execution.Ref.Revision] = revision
	held.UpdatedAt = in.At.UTC()
	if held.Current.Ref == in.Execution.Ref {
		held.Current = revision
	}
	m.tickets[in.Execution.Ref.Key] = held
	delete(m.announced, in.Execution.Ref)
	delete(m.outcomeClaims, in.Execution.Ref)
	return cloneTicket(held), nil
}
