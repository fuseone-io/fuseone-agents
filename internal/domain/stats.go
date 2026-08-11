package domain

import "time"

// RunFilter narrows a query over runs. The zero value matches everything.
type RunFilter struct {
	Scope   Scope
	AgentID AgentID
	// Since bounds the window; the zero time means no bound.
	Since time.Time
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
	// Ended is how many runs the median was computed over. Without it the
	// median is a number with no stated basis.
	Ended int64
}

// Count returns the tally for a phase, or zero.
func (s RunStats) Count(phase string) int64 { return s.ByPhase[phase] }
