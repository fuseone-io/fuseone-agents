package ledger

import (
	"encoding/json"
	"time"

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

func failureOf(s domain.Step) *domain.FailureSummary {
	if s.Kind != domain.StepParked {
		return nil
	}
	var p domain.ParkedPayload
	if err := json.Unmarshal(s.Payload, &p); err != nil || p.Failure == nil {
		return nil
	}
	return p.Failure
}

func failureCode(s domain.Step) *string {
	if f := failureOf(s); f != nil && f.Code != "" {
		return &f.Code
	}
	return nil
}

func failureProvider(s domain.Step) *string {
	if f := failureOf(s); f != nil && f.Provider != "" {
		return &f.Provider
	}
	return nil
}

func failureStatus(s domain.Step) *int {
	if f := failureOf(s); f != nil && f.Status != 0 {
		return &f.Status
	}
	return nil
}

func failureRequestID(s domain.Step) *string {
	if f := failureOf(s); f != nil && f.RequestID != "" {
		return &f.RequestID
	}
	return nil
}

func failureRetryable(s domain.Step) *bool {
	if f := failureOf(s); f != nil {
		return &f.Retryable
	}
	return nil
}

func projectPhase(s domain.Step) (phase *string, endedAt *time.Time) {
	set := func(v string) *string { return &v }
	switch s.Kind {
	case domain.StepRunStarted:
		return set("running"), nil
	case domain.StepToolCalled:
		return set("awaiting_tool"), nil
	case domain.StepToolReturned, domain.StepApprovalDecided:
		return set("running"), nil
	case domain.StepApprovalRequested:
		return set("awaiting_approval"), nil
	case domain.StepParked:
		return set("parked"), nil
	case domain.StepResumed:
		return set("running"), nil
	case domain.StepAbandoned:
		// A person ended the run. Whether it is over depends on whether
		// anything still has to be taken back first.
		var p domain.AbandonedPayload
		if err := json.Unmarshal(s.Payload, &p); err == nil && p.Compensate {
			return set("compensating"), nil
		}
		at := s.At
		return set("failed"), &at
	case domain.StepFailed:
		at := s.At
		return set("failed"), &at
	case domain.StepRunFinished:
		at := s.At
		return set("finished"), &at
	}
	return nil, nil
}

func phaseOrRunning(p *string) string {
	if p == nil {
		return "running"
	}
	return *p
}

// labelsOrEmpty keeps a nil label set out of a not-null column. pgx encodes a
// nil slice as SQL NULL, and "no labels" is an empty set, not an unknown one.
func labelsOrEmpty(l domain.Labels) []string {
	if len(l) == 0 {
		return []string{}
	}
	return []string(l)
}

func nilIfEmpty(b []byte) []byte {
	if len(b) == 0 {
		return nil
	}
	return b
}
