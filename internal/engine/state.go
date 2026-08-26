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
	// ContextArtifacts are the event-supplied references this run may read
	// through the platform-owned context tool. The set is sealed on
	// run_started and never grows from model text.
	ContextArtifacts []domain.ContextArtifact

	// Called is every tool this run has reached the far side of the Gate
	// with, in order. It is what advances a run through its declared steps:
	// the proposal moves it forward, so nothing has to judge whether a stage
	// is finished.
	Called []domain.ToolID
	// Planned records that the model has already had its first turn. Platform
	// preflight helpers, such as the initial memory lookup, only run before
	// that point; once a plan exists they must not reappear just because the
	// helper itself was skipped.
	Planned bool

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
	// ConsecutiveSkips counts recognised duplicate effects since the last call
	// that went through or the last real refusal. A duplicate is not a refusal,
	// but repeating it forever is still no progress.
	ConsecutiveSkips int

	// startedAt is the first step's instant, held so elapsed time is a fact of
	// the trail rather than of whichever worker happens to be running.
	startedAt time.Time

	// requested is the call the pending approval is about, held between the
	// request and the decision so the grant can name what it granted.
	requested *ApprovedCall

	// executed holds the idempotency keys the ledger has already recorded, so
	// a resumed run never causes the same effect twice (PRD DE-16).
	executed map[string]struct{}
}

// Committed is settled spend plus outstanding reservations. This is the figure
// the Gate compares against the budget: checking Spent alone leaves a window
// in which parallel steps overshoot the ceiling (PRD FO-01).
func (s State) Committed() domain.Consumption {
	return domain.Consumption{
		Micros:      s.Spent.Micros + s.Reserved.Micros,
		Tokens:      s.Spent.Tokens + s.Reserved.Tokens,
		ToolCalls:   s.Spent.ToolCalls,
		Steps:       s.Spent.Steps,
		WallClockMS: s.Spent.WallClockMS,
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
