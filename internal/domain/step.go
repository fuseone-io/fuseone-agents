package domain

import (
	"errors"
	"fmt"
	"time"
)

// StepKind enumerates what a ledger entry records. The set is closed: an
// unrecognised kind is a bug, not an extension point, because every projection
// over the ledger — audit trail, cost rollup, replay, state fold — must know
// how to interpret every kind.
type StepKind string

const (
	StepRunStarted        StepKind = "run_started"
	StepPlanned           StepKind = "planned"
	StepGateDecided       StepKind = "gate_decided"
	StepBudgetReserved    StepKind = "budget_reserved"
	StepToolCalled        StepKind = "tool_called"
	StepToolReturned      StepKind = "tool_returned"
	StepBudgetReconciled  StepKind = "budget_reconciled"
	StepApprovalRequested StepKind = "approval_requested"
	StepApprovalDecided   StepKind = "approval_decided"
	StepCompensated       StepKind = "compensated"
	StepFailed            StepKind = "failed"
	StepParked            StepKind = "parked"
	StepRunFinished       StepKind = "run_finished"
)

var stepKinds = map[StepKind]bool{
	StepRunStarted: true, StepPlanned: true, StepGateDecided: true,
	StepBudgetReserved: true, StepToolCalled: true, StepToolReturned: true,
	StepBudgetReconciled: true, StepApprovalRequested: true,
	StepApprovalDecided: true, StepCompensated: true, StepFailed: true,
	StepParked: true, StepRunFinished: true,
}

func (k StepKind) Valid() bool { return stepKinds[k] }

// Terminal reports whether the kind ends the run.
func (k StepKind) Terminal() bool {
	return k == StepRunFinished || k == StepParked
}

var (
	ErrInvalidKind  = errors.New("invalid step kind")
	ErrInvalidScope = errors.New("invalid scope")
	ErrInvalidSeq   = errors.New("sequence must start at 1")
	// ErrSeqGap is a step missing from the middle, which is the tamper that
	// matters: every remaining step is internally valid and only the links
	// give it away. Its own error because "must start at 1" sent whoever read
	// it looking at the beginning of a chain whose beginning is fine.
	ErrSeqGap = errors.New("a step is missing from the chain")
	ErrNoRun  = errors.New("run id is required")
)

// FirstSeq is the sequence number of a run's opening step.
const FirstSeq int64 = 1

// Step is one immutable entry in the ledger.
//
// Nothing here is ever updated or deleted (PRD AU-01). A correction is a new
// step, never an edit. Every projection the product sells — the audit trail,
// the cost ledger, replay, the regression corpus and the run state itself —
// is derived from this one record type.
type Step struct {
	RunID RunID
	// Seq is monotonic within a run, starting at FirstSeq. Uniqueness of
	// (RunID, Seq) at the storage layer is what enforces single-writer
	// (PRD NF-15): a second writer racing for the same sequence loses.
	Seq   int64
	Kind  StepKind
	Scope Scope

	AgentID   AgentID
	VersionID VersionID
	// OnBehalfOf is the delegating human. The agent is a principal in its own
	// right, and the trail always records the pair (PRD AU-05).
	OnBehalfOf UserID

	// Payload is canonical JSON. It is hashed as raw bytes, so whatever
	// produced it must serialise deterministically.
	Payload []byte
	Labels  Labels
	Cost    Cost

	// IdemKey deduplicates side effects across retries and resumes. Empty for
	// steps that cause no external effect.
	IdemKey string
	// PolicyHash pins the policy version evaluated for this step, which is
	// what makes counterfactual replay possible (PRD AU-08).
	PolicyHash string

	At time.Time

	PrevHash []byte
	Hash     []byte
}

// NewStep builds a step, seals it into the chain after prev, and returns it.
//
// Passing a zero prev starts a chain, which is only valid at FirstSeq.
func NewStep(prev *Step, s Step) (Step, error) {
	if s.RunID == "" {
		return Step{}, ErrNoRun
	}
	if !s.Kind.Valid() {
		return Step{}, ErrInvalidKind
	}
	if !s.Scope.Valid() {
		return Step{}, ErrInvalidScope
	}

	if prev == nil {
		s.Seq = FirstSeq
		s.PrevHash = nil
	} else {
		s.Seq = prev.Seq + 1
		s.PrevHash = prev.Hash
	}
	if s.Seq < FirstSeq {
		return Step{}, ErrInvalidSeq
	}

	// Postgres timestamptz keeps microseconds. Truncating here means a step
	// hashed in memory still verifies after a round trip through storage.
	s.At = s.At.UTC().Truncate(time.Microsecond)
	s.Labels = NewLabels(s.Labels...)
	// Canonicalised before sealing so the digest survives any store that
	// reshapes JSON on the way in and out.
	s.Payload = CanonicalJSON(s.Payload)
	s.Hash = s.computeHash()
	return s, nil
}

// VerifyLink reports whether s is correctly sealed against prev.
func (s Step) VerifyLink(prev *Step) error {
	switch {
	case prev == nil && s.Seq != FirstSeq:
		return ErrInvalidSeq
	case prev != nil && s.Seq != prev.Seq+1:
		return fmt.Errorf("%w: %s jumps from %d to %d", ErrSeqGap, s.RunID, prev.Seq, s.Seq)
	case prev != nil && !equalBytes(s.PrevHash, prev.Hash):
		return ErrChainBroken
	case prev == nil && len(s.PrevHash) != 0:
		return ErrChainBroken
	case !equalBytes(s.Hash, s.computeHash()):
		return ErrHashMismatch
	}
	return nil
}
