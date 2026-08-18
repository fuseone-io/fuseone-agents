package domain

import "time"

// RunSummary is a run as a list shows it.
//
// Read from the projection rather than folded from steps: a page of fifty runs
// that folds every run in the ledger to find them costs more with every week
// the installation runs, which is the one thing an append-only record is
// guaranteed to do.
type RunSummary struct {
	RunID      RunID
	Scope      Scope
	AgentID    AgentID
	VersionID  VersionID
	OnBehalfOf UserID

	Phase string
	Seq   int64

	Cost           Cost
	ReservedMicros int64
	ToolCalls      int64
	Labels         Labels

	StartedAt time.Time
	// EndedAt is zero while the run is still going.
	EndedAt time.Time
	// UpdatedAt is when the run last moved. For a run waiting on a person it
	// is the moment it stopped to ask, which is what an inbox sorts by.
	UpdatedAt time.Time

	// PendingApproval is set while the run waits on a person.
	PendingApproval *PendingApprovalSummary

	// Failure is set when the supervisor parked the run for a typed cause.
	// The raw last_error remains in the queue projection, but a screen or
	// metric reads this instead: stable code, provider, status and request id.
	Failure *FailureSummary
}

// PendingApprovalSummary is the suspended action, denormalised so an inbox is
// one indexed read rather than a fold of every suspended run.
type PendingApprovalSummary struct {
	Tool   ToolID
	Rule   string
	Reason string
	AtSeq  int64
}

// CostBucket is one row of a cost rollup.
type CostBucket struct {
	Key  string
	Cost Cost
	Runs int64
}
