// Package engine interprets the fixed agent loop over the ledger.
//
// The loop has four states — plan, gate, execute, append — and the run's whole
// state is a fold over its recorded steps. That is what makes resume free: a
// worker that dies mid-run reloads the ledger and continues at the identical
// state, with no separate checkpoint to keep in sync (PRD NF-02).
package engine

import (
	"time"

	"context"
	"encoding/json"
	"fmt"

	"github.com/fuseone/agents/internal/domain"
)

// Ledger is the storage this package needs. It is declared here, by the
// consumer; implementations in package ledger satisfy it structurally, so
// ledger never imports engine.
type Ledger interface {
	Append(ctx context.Context, s domain.Step) (domain.Step, error)
	Read(ctx context.Context, runID domain.RunID, fromSeq int64) ([]domain.Step, error)
	Head(ctx context.Context, runID domain.RunID) (domain.Step, error)
}

// Phase is what the run is doing right now.
type Phase uint8

const (
	PhaseUnstarted Phase = iota
	PhaseRunning
	PhaseAwaitingApproval
	PhaseAwaitingTool
	PhaseParked
	PhaseFinished
	// PhaseCompensating is a run a person ended whose undoing is still being
	// carried out. Claimable, because the undos are real tool calls and the
	// worker pool is what runs those.
	PhaseCompensating
	// PhaseFailed is the other ending. Parking is a pause and resumes; this
	// one does not — it is the run somebody decided cannot go on, after what
	// it left standing was compensated (PRD SE-08).
	PhaseFailed
)

var phaseNames = [...]string{"unstarted", "running", "awaiting_approval", "awaiting_tool", "parked", "finished", "compensating", "failed"}

func (p Phase) String() string {
	if int(p) < len(phaseNames) {
		return phaseNames[p]
	}
	return fmt.Sprintf("phase(%d)", uint8(p))
}

// PendingApproval is a suspended action waiting on a human.
type PendingApproval struct {
	Tool   domain.ToolID
	Rule   string
	Reason string
	AtSeq  int64
	// Effect and At are what an approver decides on: what the call does to the
	// world, and how long it has been waiting for them.
	Effect domain.Effect
	At     time.Time
}

// ApprovedCall is the exact call a person cleared.
//
// Exact on purpose. A grant is for one tool with one set of arguments, and
// executing anything else on the strength of it would be the platform doing
// something nobody agreed to — the failure this whole queue exists to prevent.
type ApprovedCall struct {
	Tool       domain.ToolID
	ArgsRef    string
	ArgsDigest string
	// AtSeq is the approval_requested step it answers.
	AtSeq int64
}

// State is the run reconstructed from its ledger. Every field is derived; none
// is authoritative on its own.
type State struct {
	RunID      domain.RunID
	Scope      domain.Scope
	AgentID    domain.AgentID
	VersionID  domain.VersionID
	OnBehalfOf domain.UserID

	Seq   int64
	Phase Phase

	// Spent is settled consumption; Reserved is outstanding reservations that
	// have not been reconciled yet. The Gate checks Committed, never Spent.
	Spent    domain.Consumption
	Reserved domain.Consumption

	// Labels is the accumulated taint of the run context. It only grows.
	Labels domain.Labels

	// Called is every tool this run has reached the far side of the Gate
	// with, in order. It is what advances a run through its declared steps:
	// the proposal moves it forward, so nothing has to judge whether a stage
	// is finished.
	Called []domain.ToolID

	PendingApproval *PendingApproval
	// Approved is a call a person cleared and the loop has not made yet.
	//
	// It exists because an approval has to survive the turn that granted it.
	// Without it the loop replans, the model proposes something else, and the
	// Gate — which never saw the grant — asks again: the approval decided
	// nothing and the run loops until it parks.
	Approved *ApprovedCall
	// PendingTool is set while a tool call is recorded but its result is not.
	// A run loaded in that shape crashed mid-call, and the outcome is unknown.
	PendingTool domain.ToolID

	// ConsecutiveBlocks counts refusals since the last call that went through.
	// A refusal is fed back to the planner so it can choose differently; this
	// is how the platform notices that it did not.
	ConsecutiveBlocks int

	// requested is the call the pending approval is about, held between the
	// request and the decision so the grant can name what it granted.
	requested *ApprovedCall

	// executed holds the idempotency keys the ledger has already recorded, so
	// a resumed run never causes the same effect twice (PRD DE-16).
	executed map[string]struct{}
}

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
		s.RunID = step.RunID
		s.Scope = step.Scope
		s.AgentID = step.AgentID
		s.VersionID = step.VersionID
		s.OnBehalfOf = step.OnBehalfOf
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
		s.Phase = PhaseRunning

	case domain.StepRunFinished:
		s.PendingApproval, s.requested, s.Approved = nil, nil, nil
		s.Phase = PhaseFinished

	case domain.StepGateDecided:
		var p domain.GateDecidedPayload
		if err := decode(step, &p); err != nil {
			return err
		}
		if !p.Verdict.Executable() && p.Verdict != domain.VerdictRequireApproval {
			s.ConsecutiveBlocks++
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

	case domain.StepPlanned, domain.StepCompensated:
		// Recorded for the trail; they carry no state transition of their own.

	default:
		return fmt.Errorf("engine: unhandled step kind %q", step.Kind)
	}
	return nil
}

// Committed is settled spend plus outstanding reservations. This is the figure
// the Gate compares against the budget: checking Spent alone leaves a window
// in which parallel steps overshoot the ceiling (PRD FO-01).
func (s State) Committed() domain.Consumption {
	return domain.Consumption{
		Micros:    s.Spent.Micros + s.Reserved.Micros,
		Tokens:    s.Spent.Tokens + s.Reserved.Tokens,
		ToolCalls: s.Spent.ToolCalls,
		Steps:     s.Spent.Steps,
	}
}

// AlreadyExecuted reports whether the ledger already records this idempotency
// key, meaning the effect happened and must not be repeated.
func (s State) AlreadyExecuted(key string) bool {
	_, ok := s.executed[key]
	return ok
}

// Terminal reports whether the run has ended for good. Parked runs are not
// terminal: they are suspended and resumable.
func (s State) Terminal() bool {
	return s.Phase == PhaseFinished || s.Phase == PhaseFailed
}

// Resumable reports whether a worker may pick this run up and continue it.
func (s State) Resumable() bool {
	switch s.Phase {
	case PhaseRunning, PhaseAwaitingTool, PhaseParked, PhaseCompensating:
		return true
	}
	return false
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
