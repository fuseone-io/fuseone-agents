package ledger

import (
	"encoding/json"

	"github.com/fuseone/agents/internal/domain"
)

// Deltas the runs projection applies for a single step.
//
// These mirror engine.State.Apply. The contract suite folds the same steps in
// Go and compares, so the two cannot drift apart unnoticed.

// reservationDelta is what the step adds to, or releases from, outstanding
// reservations. Reserved budget counts against the ceiling until reconciled,
// which is what stops parallel steps overshooting it (PRD FO-01).
func reservationDelta(s domain.Step) int64 {
	switch s.Kind {
	case domain.StepBudgetReserved:
		var p domain.BudgetReservedPayload
		decodePayload(s, &p)
		return p.Micros
	case domain.StepBudgetReconciled:
		var p domain.BudgetReconciledPayload
		decodePayload(s, &p)
		return -p.ReleasedMicros
	}
	return 0
}

func toolCallDelta(s domain.Step) int64 {
	if s.Kind == domain.StepToolCalled {
		return 1
	}
	return 0
}

// The pending-approval columns are set on every upsert: the request fills
// them, any later step clears them. Leaving them stale would keep a decided
// run in the approval inbox for ever.
func pendingTool(s domain.Step) *string {
	if s.Kind != domain.StepApprovalRequested {
		return nil
	}
	var p domain.ApprovalRequestedPayload
	decodePayload(s, &p)
	return strPtr(string(p.Tool))
}

func pendingRule(s domain.Step) *string {
	if s.Kind != domain.StepApprovalRequested {
		return nil
	}
	var p domain.ApprovalRequestedPayload
	decodePayload(s, &p)
	return strPtr(p.Rule)
}

func pendingReason(s domain.Step) *string {
	if s.Kind != domain.StepApprovalRequested {
		return nil
	}
	var p domain.ApprovalRequestedPayload
	decodePayload(s, &p)
	return strPtr(p.Reason)
}

func pendingAtSeq(s domain.Step) *int64 {
	if s.Kind != domain.StepApprovalRequested {
		return nil
	}
	seq := s.Seq
	return &seq
}

// decodePayload is best-effort: a payload that will not decode leaves the
// projection unchanged rather than failing the append. The ledger row is
// already written and is the source of truth; the projection can be rebuilt.
func decodePayload(s domain.Step, into any) {
	if len(s.Payload) == 0 {
		return
	}
	_ = json.Unmarshal(s.Payload, into)
}

func strPtr(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}
