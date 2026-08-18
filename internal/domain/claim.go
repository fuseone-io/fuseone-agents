package domain

import (
	"errors"
	"time"
)

// ErrNoClaimableRun means the queue is empty right now. It is a normal
// outcome, not a failure: a worker sees it whenever it is faster than the
// work arriving.
var ErrNoClaimableRun = errors.New("no claimable run")

// Claim is a run leased to one worker for one turn.
//
// The lease is time-bound rather than a held lock. Advancing a run makes a
// model call and a tool call, and holding a database transaction across those
// would pin a connection per in-flight agent, turning a slow upstream into a
// database outage. A worker that dies simply stops renewing.
type Claim struct {
	RunID      RunID
	Scope      Scope
	AgentID    AgentID
	VersionID  VersionID
	OnBehalfOf UserID
	// Phase is what the run was doing when it was claimed. The worker needs it
	// because not every claimable run wants advancing: one somebody abandoned
	// wants undoing, and that is a different job entirely.
	Phase string
	// Attempts is the number of consecutive failures preceding this turn. It
	// resets on any turn that makes progress.
	Attempts int
	// LeasedUntil is when another worker may take this run over.
	LeasedUntil time.Time
}

// ClaimOutcome is how a turn ended, and what the queue should do next.
type ClaimOutcome struct {
	// NextAttemptAt is when the run becomes claimable again. Zero means
	// immediately — the normal case, since one turn rarely finishes a run.
	NextAttemptAt time.Time
	// Err is set when the turn failed. A non-nil Err increments the attempt
	// count; success resets it.
	Err error
	// Failure is the stable, low-cardinality part of Err, when the caller can
	// classify it. It feeds operational views; raw error text stays out of
	// dashboards and aggregate labels.
	Failure *FailureSummary
	// Parked withdraws the run from the queue until a human intervenes. The
	// alternative — retrying for ever — burns budget and hides the fault
	// (PRD NF-14).
	Parked bool
}

// Failed reports whether the turn ended badly.
func (o ClaimOutcome) Failed() bool { return o.Err != nil }

// Reason renders the failure for the console, or an empty string on success.
func (o ClaimOutcome) Reason() string {
	if o.Err == nil {
		return ""
	}
	return o.Err.Error()
}
