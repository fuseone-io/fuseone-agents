package domain

import "time"

// RunFilter narrows a query over runs. The zero value matches everything.
type RunFilter struct {
	Scope   Scope
	AgentID AgentID

	// Scopes narrows to any of several, which is what a caller who holds a
	// permission in more than one area needs. It is how a listing is filtered
	// at the query rather than read whole and discarded — the difference
	// between showing somebody their areas and showing them everyone's
	// (PRD NF-06).
	//
	// A scope with no area covers its whole company, matching Scope.Contains.
	Scopes []Scope
	// Since and Until bound the window; a zero time means no bound on that
	// side. Cost asks for both because a rollup without an upper bound is a
	// figure that changes while you read it.
	Since time.Time
	Until time.Time

	// After resumes a previous page, at the position that page ended. Nil
	// starts at the newest run.
	After *RunCursor

	// Search matches the run or agent identifier. It is a filter like any
	// other rather than something applied to a page: a search that only looked
	// at the rows already loaded would answer differently depending on how
	// many the caller asked for.
	Search string
}

// RunStats answers a question about many runs at once.
//
// It exists because the honest alternative is worse: a console that derives
// "97% concluded" from whichever page happened to load is stating a fact it
// cannot support, and this product's whole claim is that its numbers mean
// something.
type RunStats struct {
	// Total is how many runs matched, not how many were returned.
	Total int64
	// ByPhase counts runs per phase. Phases with no runs are absent rather
	// than zero, so a reader can tell "none" from "not measured".
	ByPhase map[string]int64
	// MedianDurationMS covers runs that have ended. A median rather than a
	// mean: one run that parked overnight would drag an average somewhere no
	// individual run ever was.
	MedianDurationMS int64
	// P95DurationMS is the slow tail, over the same runs as the median. On its
	// own a median says nothing about the runs people actually complain about,
	// and the two together are what say whether a change helped.
	P95DurationMS int64
	// Ended is how many runs the median was computed over. Without it the
	// median is a number with no stated basis.
	Ended int64
}

// Count returns the tally for a phase, or zero.
func (s RunStats) Count(phase string) int64 { return s.ByPhase[phase] }

// ThroughputBucket is one interval of a run count, split by what became of the
// runs that started in it.
//
// Runs are bucketed by when they started, and counted under the phase they are
// in now. That is the honest reading of "how is today going": a run that
// started at nine and is still waiting belongs to nine o'clock and to the
// waiting column, not to whenever it eventually ends.
type ThroughputBucket struct {
	At      time.Time
	ByPhase map[string]int64
	// ByAgent is the same hour split by who ran. It travels with the phase
	// split rather than in its own request because every screen that wants one
	// wants the other, and two queries over the same rows can disagree if a
	// run finishes between them.
	ByAgent map[string]int64
	Total   int64
	// Micros is what the hour cost, so a spend figure and a run count are read
	// off the same rows.
	Micros int64
}

// RecordedDecision is a Gate decision as the ledger kept it — the verdict
// together with the run, agent and moment it applied to.
//
// Distinct from domain.Decision, which is what the Gate returns before
// anything is written: that one carries the arguments a constraint rewrote and
// the policy hash, and this one carries where it landed. A feed of these is a
// reader watching whether the installation's rules are doing anything.
type RecordedDecision struct {
	RunID   RunID
	Seq     int64
	At      time.Time
	Scope   Scope
	AgentID AgentID
	Tool    ToolID
	Verdict Verdict
	Rule    string
	// PolicyCode names the authored rule that produced this, when one did.
	// Counting what a policy actually decided reads the trail rather than a
	// counter, because a counter drifts and the trail is what happened.
	PolicyCode string
	// Effect and Labels are what a draft rule is replayed against. The
	// arguments are not here: a blocked call never stored any, so a rule that
	// reads them cannot be replayed against every decision — and a simulation
	// that quietly treated missing arguments as empty would report no matches
	// and read as reassurance.
	Effect Effect
	Labels Labels
}
