// Package engine interprets the fixed agent loop over the ledger.
//
// The loop has four states — plan, gate, execute, append — and the run's whole
// state is a fold over its recorded steps. That is what makes resume free: a
// worker that dies mid-run reloads the ledger and continues at the identical
// state, with no separate checkpoint to keep in sync (PRD NF-02).
package engine

import (
	"encoding/json"
	"fmt"

	"github.com/fuseone/agents/internal/domain"
)

/*
Folding the ledger into what a run is now.

Every projection in this platform is one of these: the trail, the cost, the
diagram, the report. None of them is stored, all of them are derived, and that
is what makes the record the only thing anybody has to trust (PRD AU-02).
*/
// Fold replays steps into a State. Steps must be contiguous and in order.
func Fold(steps []domain.Step) (State, error) {
	var s State
	for _, step := range steps {
		if err := s.Apply(step); err != nil {
			return State{}, err
		}
	}
	return s, nil
}

// Apply advances the state by exactly one step.
func (s *State) Apply(step domain.Step) error {
	if step.Seq != s.Seq+1 {
		return fmt.Errorf("engine: out-of-order step: got seq %d, want %d", step.Seq, s.Seq+1)
	}
	s.Seq = step.Seq

	// Cost rides on the step itself, whatever the kind, so accounting can
	// never drift from the record it is derived from.
	s.Spent.Micros += step.Cost.Micros
	s.Spent.Tokens += step.Cost.TotalTokens()
	s.Spent.Steps++
	s.Labels = s.Labels.Union(step.Labels)

	// How long the run has been going, measured from its own trail rather
	// than from a clock the worker holds. A run that crossed a restart, or
	// that resumed hours after parking, has to arrive at the Gate with its
	// real elapsed time — and the only record of that is the instants the
	// steps were sealed with (PRD FO-03).
	if s.startedAt.IsZero() {
		s.startedAt = step.At
	}
	s.Spent.WallClockMS = step.At.Sub(s.startedAt).Milliseconds()

	if step.IdemKey != "" {
		if s.executed == nil {
			s.executed = make(map[string]struct{})
		}
		s.executed[step.IdemKey] = struct{}{}
	}

	return s.applyKind(step)
}

func (s *State) applyKind(step domain.Step) error {
	switch step.Kind {
	case domain.StepRunStarted:
		var p domain.RunStartedPayload
		if err := decode(step, &p); err != nil {
			return err
		}
		s.RunID = step.RunID
		s.Scope = step.Scope
		s.AgentID = step.AgentID
		s.VersionID = step.VersionID
		s.OnBehalfOf = step.OnBehalfOf
		s.ContextArtifacts = p.ContextArtifacts
		s.Phase = PhaseRunning

	case domain.StepBudgetReserved:
		var p domain.BudgetReservedPayload
		if err := decode(step, &p); err != nil {
			return err
		}
		s.Reserved.Micros += p.Micros
		s.Reserved.Tokens += p.Tokens

	case domain.StepBudgetReconciled:
		var p domain.BudgetReconciledPayload
		if err := decode(step, &p); err != nil {
			return err
		}
		s.Reserved.Micros -= p.ReleasedMicros
		s.Reserved.Tokens -= p.ReleasedTokens

	case domain.StepToolCalled:
		// The grant is spent. Holding it would let a resumed run make the
		// approved call a second time.
		s.Approved = nil
		var p domain.ToolCalledPayload
		if err := decode(step, &p); err != nil {
			return err
		}
		s.Spent.ToolCalls++
		s.Called = append(s.Called, p.Tool)
		s.PendingTool = p.Tool
		s.Phase = PhaseAwaitingTool
		// A call that reached the Gate's far side is progress, whatever it
		// returns: the planner is no longer stuck on a refusal.
		s.ConsecutiveBlocks = 0
		s.ConsecutiveSkips = 0

	case domain.StepToolReturned:
		s.PendingTool = ""
		s.Phase = PhaseRunning

	case domain.StepApprovalRequested:
		var p domain.ApprovalRequestedPayload
		if err := decode(step, &p); err != nil {
			return err
		}
		s.PendingApproval = &PendingApproval{
			Tool: p.Tool, Rule: p.Rule, Reason: p.Reason,
			AtSeq: step.Seq, Effect: p.Effect, At: step.At,
		}
		s.requested = &ApprovedCall{
			Tool: p.Tool, ArgsRef: p.ArgsRef, ArgsDigest: p.ArgsDigest, AtSeq: step.Seq,
		}
		s.Phase = PhaseAwaitingApproval

	case domain.StepApprovalDecided:
		var p domain.ApprovalDecidedPayload
		if err := decode(step, &p); err != nil {
			return err
		}
		// A refusal leaves nothing to carry forward. The planner sees it in
		// the transcript and chooses again, which is the point of asking.
		if p.Approved {
			s.Approved = s.requested
		}
		s.PendingApproval, s.requested = nil, nil
		s.Phase = PhaseRunning

	case domain.StepParked:
		s.Phase = PhaseParked

	case domain.StepResumed:
		// Back to running at the sequence it stopped at. Nothing is replayed:
		// the run's spend, its calls and its taint are all the fold of the
		// steps before this one, and they stand.
		//
		// The refusal count is the exception, because it is not a fact about
		// the run — it is evidence about a world that has just changed. The
		// person resuming has said they fixed the thing; carrying the count
		// would park the run again on its first refusal instead of giving it
		// the attempts the supervision policy allows.
		s.ConsecutiveBlocks = 0
		s.ConsecutiveSkips = 0
		s.Phase = PhaseRunning

	case domain.StepRunFinished:
		s.PendingApproval, s.requested, s.Approved = nil, nil, nil
		s.Phase = PhaseFinished

	case domain.StepGateDecided:
		var p domain.GateDecidedPayload
		if err := decode(step, &p); err != nil {
			return err
		}
		switch {
		case p.Verdict == domain.VerdictDuplicate:
			s.ConsecutiveSkips++
			s.ConsecutiveBlocks = 0
		case !p.Verdict.Executable() && p.Verdict != domain.VerdictRequireApproval:
			s.ConsecutiveBlocks++
			s.ConsecutiveSkips = 0
		}

	case domain.StepAbandoned:
		var p domain.AbandonedPayload
		if err := decode(step, &p); err != nil {
			return err
		}
		// Nothing left to decide. A run that ended still carrying a pending
		// approval asks somebody to rule on a call that will never happen,
		// and the console offers them the button to do it.
		s.PendingApproval, s.requested, s.Approved = nil, nil, nil
		// The decision is made either way. What differs is whether anything
		// still has to be undone before the run can be called over.
		s.Phase = PhaseFailed
		if p.Compensate {
			s.Phase = PhaseCompensating
		}

	case domain.StepFailed:
		s.PendingApproval, s.requested, s.Approved = nil, nil, nil
		s.Phase = PhaseFailed

	case domain.StepPlanned:
		s.Planned = true

	case domain.StepCompensated:
		// Recorded for the trail; they carry no state transition of their own.

	default:
		return fmt.Errorf("engine: unhandled step kind %q", step.Kind)
	}
	return nil
}

func decode(step domain.Step, into any) error {
	if len(step.Payload) == 0 {
		return nil
	}
	if err := json.Unmarshal(step.Payload, into); err != nil {
		return fmt.Errorf("engine: decode %s payload at seq %d: %w", step.Kind, step.Seq, err)
	}
	return nil
}
