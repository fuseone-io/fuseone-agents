package domain

import "time"

// Expectation is one thing an author says should be true of a case.
//
// It is the shape a correction takes (PRD FU-11), and the reason it has a
// shape at all is FU-12: a correction becomes a regression case, and every
// future version is run against it. Prose cannot do that. "It should have
// asked me" is a sentence a person understands and nothing can check, so what
// is recorded is the checkable half of it.
//
// The four kinds are what the fold of a run can answer without inventing
// anything: where the case ended, and what it would have done on the way.
type Expectation struct {
	Kind ExpectationKind `json:"kind"`
	// Step anchors it to one declared step of the agent rather than to the
	// whole of it (PRD FU-13). A correction about the reply step must not
	// start failing because the lookup step changed.
	//
	// Empty means the run as a whole, which is the only sensible anchor for an
	// agent that declares no steps.
	Step string `json:"step,omitempty"`
	// Value is the tool, or the settled state, depending on the kind.
	Value string `json:"value,omitempty"`
}

type ExpectationKind string

const (
	// ExpectSettles is where the case should end: finished, parked, or
	// waiting on a person.
	ExpectSettles ExpectationKind = "settles"
	// ExpectCalls is a tool the agent should reach.
	ExpectCalls ExpectationKind = "calls"
	// ExpectNeverCalls is a tool it must not reach. The most common
	// correction there is, and the one worth the whole mechanism.
	ExpectNeverCalls ExpectationKind = "never_calls"
	// ExpectAsks is a tool it should stop and ask a person about. Value is
	// optional: asking about anything at all is sometimes the whole point.
	ExpectAsks ExpectationKind = "asks"

	// ExpectCostsAtMost and ExpectWithinSteps are ceilings, in micros and in
	// steps.
	//
	// Every other kind is about shape — which tool, which ending, whether a
	// person was asked — so a version that did all of that correctly and spent
	// three times as much passed green. The regression nobody could express
	// was the one that shows up on the invoice.
	//
	// Ceilings and not targets: a case that got cheaper is not a failure, and
	// asserting an exact figure would break the corpus every time a provider
	// changed its tokeniser.
	ExpectCostsAtMost ExpectationKind = "costs_at_most"
	ExpectWithinSteps ExpectationKind = "within_steps"

	// ExpectCallsBefore is one tool reached before another, as "first,second".
	//
	// Every other kind is about a set — which tools, which ending, how much.
	// None of them can say *before*, and "it replied without looking the
	// customer up first" is a correction people actually make: the agent did
	// both of the right things in the wrong order, so every other assertion
	// holds and the run was still wrong.
	ExpectCallsBefore ExpectationKind = "calls_before"
)

func (k ExpectationKind) Valid() bool {
	switch k {
	case ExpectSettles, ExpectCalls, ExpectNeverCalls, ExpectAsks,
		ExpectCostsAtMost, ExpectWithinSteps, ExpectCallsBefore:
		return true
	}
	return false
}

// RegressionCase is one correction, kept to be re-run.
//
// It holds the occurrence and what must be true of it, and nothing about how
// it was decided: the run it came from is recorded so somebody can go and
// read it, and may well be purged long before this is.
type RegressionCase struct {
	ID           string
	Agent        AgentID
	Scope        Scope
	InputRef     string
	Expectations []Expectation
	// FromRun is the run the author was looking at. Provenance, not a
	// dependency — the corpus keeps its own copy of the occurrence.
	FromRun RunID
	Note    string

	CreatedBy UserID
	CreatedAt time.Time
}
