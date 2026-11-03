package domain

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

/*
The ledger's weekly measures of the factory (#201): per ISO week and agent,
how many runs opened, how many cards they worked, what they asked people and
how people answered, and where their cost went.

A week starts on Monday at 00:00 UTC, whatever zone the instant was read in.
A run belongs to the week it opened in, and everything it asked counts there,
however late it was answered — so a closed week does not change when an old
run is decided.

Counts, identifiers and instants only. What a proposal carried — its
arguments, its reasons, the note on its decision — never leaves the store.
*/

const (
	// MinMeasureWeeks and MaxMeasureWeeks bound how far back the measures
	// read: half a year of weeks is the widest window one request may ask.
	MinMeasureWeeks = 1
	MaxMeasureWeeks = 26
	// DefaultMeasureWeeks is the window when none is asked for.
	DefaultMeasureWeeks = 8
)

// ErrMeasureWeeks means the window asked for is outside the bounds.
var ErrMeasureWeeks = errors.New("weeks must be between 1 and 26")

const weekLength = 7 * 24 * time.Hour

// WeekOf is the Monday 00:00 UTC that starts the ISO week t falls in.
func WeekOf(t time.Time) time.Time {
	u := t.UTC()
	day := time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC)
	sinceMonday := (int(day.Weekday()) + 6) % 7
	return day.AddDate(0, 0, -sinceMonday)
}

// WeekComplete reports whether the week starting at start had ended by now.
func WeekComplete(start, now time.Time) bool {
	return !now.Before(start.Add(weekLength))
}

// WorkItemWindow is a span of whole weeks: From is a Monday, To the Monday
// after the last week, exclusive.
type WorkItemWindow struct {
	From, To time.Time
}

// MeasureWindow is the weeks weeks ending with the one now falls in.
func MeasureWindow(now time.Time, weeks int) (WorkItemWindow, error) {
	if weeks < MinMeasureWeeks || weeks > MaxMeasureWeeks {
		return WorkItemWindow{}, fmt.Errorf("%w: %d", ErrMeasureWeeks, weeks)
	}
	to := WeekOf(now).Add(weekLength)
	return WorkItemWindow{From: to.Add(-time.Duration(weeks) * weekLength), To: to}, nil
}

// StandingApprovalNotePrefix opens the note of a decision the engine wrote
// under a standing approval, on a person's behalf (#192).
const StandingApprovalNotePrefix = "standing approval "

// IsStandingDecision reports whether a decision is an approval a standing
// grant gave.
func IsStandingDecision(d ApprovalDecidedPayload) bool {
	return d.Approved && strings.HasPrefix(d.Note, StandingApprovalNotePrefix)
}

// WorkItemProposals is what a week's runs asked people and how they were
// answered. Standing approvals count in Approved too. A refusal counts in
// RevisedAfterRefusal when the run later proposed the same tool with other
// arguments, and in RepeatedAfterRefusal when it proposed the same
// arguments again — each refusal at most once in each.
type WorkItemProposals struct {
	Requested            int
	Approved             int
	Refused              int
	Abandoned            int
	Pending              int
	RevisedAfterRefusal  int
	RepeatedAfterRefusal int
	Standing             int
}

func (a WorkItemProposals) plus(b WorkItemProposals) WorkItemProposals {
	return WorkItemProposals{
		Requested: a.Requested + b.Requested, Approved: a.Approved + b.Approved,
		Refused: a.Refused + b.Refused, Abandoned: a.Abandoned + b.Abandoned,
		Pending: a.Pending + b.Pending, Standing: a.Standing + b.Standing,
		RevisedAfterRefusal:  a.RevisedAfterRefusal + b.RevisedAfterRefusal,
		RepeatedAfterRefusal: a.RepeatedAfterRefusal + b.RepeatedAfterRefusal,
	}
}

// WorkItemWeekCost is where a week's cost went. The three parts add up to
// the cost of every run the row counts:
//   - OnCardsMicros: runs that claimed a card or a pull request linked to one;
//   - InheritedMicros: runs on a card only by how they were opened — an event
//     or a GitHub delivery — which cost no card anything;
//   - OffCardMicros: runs on no card at all.
type WorkItemWeekCost struct {
	OnCardsMicros   int64
	InheritedMicros int64
	OffCardMicros   int64
}

func (a WorkItemWeekCost) plus(b WorkItemWeekCost) WorkItemWeekCost {
	return WorkItemWeekCost{
		OnCardsMicros:   a.OnCardsMicros + b.OnCardsMicros,
		InheritedMicros: a.InheritedMicros + b.InheritedMicros,
		OffCardMicros:   a.OffCardMicros + b.OffCardMicros,
	}
}

// WorkItemWeekShare is one agent's runs of one week in one scope: the unit
// a store aggregates. Cards are the distinct cards those runs claimed,
// resolved to their roots; CostReadable is whether the caller may read this
// scope's cost, decided by the store from the caller's cost scopes.
type WorkItemWeekShare struct {
	Week         time.Time
	AgentID      AgentID
	Scope        Scope
	Runs         int
	OpenRuns     int
	Cards        []WorkItemRef
	Proposals    WorkItemProposals
	Cost         WorkItemWeekCost
	CostReadable bool
}

// WorkItemWeek is one agent's week. Cost is nil unless the caller may read
// the cost of every scope the row folds — all three parts together or none.
type WorkItemWeek struct {
	Week      time.Time
	AgentID   AgentID
	Runs      int
	OpenRuns  int
	Cards     int
	Proposals WorkItemProposals
	Cost      *WorkItemWeekCost
}

type weekKey struct {
	week  time.Time
	agent AgentID
}

type weekFold struct {
	row      WorkItemWeek
	cost     WorkItemWeekCost
	readable bool
	cards    map[WorkItemRef]bool
}

// RoleWeeks folds per-scope shares into one row per week and agent, oldest
// week first, agents in order within a week.
//
// A row's cost is all of its scopes or absent: summing only the scopes the
// caller may read would print a smaller number that looks complete.
func RoleWeeks(shares []WorkItemWeekShare) []WorkItemWeek {
	folds := map[weekKey]*weekFold{}
	for _, s := range shares {
		key := weekKey{s.Week.UTC(), s.AgentID}
		f, ok := folds[key]
		if !ok {
			f = &weekFold{row: WorkItemWeek{Week: key.week, AgentID: s.AgentID}, readable: true, cards: map[WorkItemRef]bool{}}
			folds[key] = f
		}
		f.row.Runs += s.Runs
		f.row.OpenRuns += s.OpenRuns
		f.row.Proposals = f.row.Proposals.plus(s.Proposals)
		f.cost = f.cost.plus(s.Cost)
		f.readable = f.readable && s.CostReadable
		for _, c := range s.Cards {
			f.cards[c] = true
		}
	}
	out := make([]WorkItemWeek, 0, len(folds))
	for _, f := range folds {
		row := f.row
		row.Cards = len(f.cards)
		if f.readable {
			cost := f.cost
			row.Cost = &cost
		}
		out = append(out, row)
	}
	slices.SortFunc(out, func(a, b WorkItemWeek) int {
		return cmp.Or(a.Week.Compare(b.Week), strings.Compare(string(a.AgentID), string(b.AgentID)))
	})
	return out
}

// UnavailableMeasure is a measure #201 asks for that this endpoint does not
// answer, and why — said once, never as a zero that reads as a fact.
type UnavailableMeasure struct {
	Measure string
	Reason  string
	Detail  string
}

// UnavailableWorkItemMeasures is what the ledger cannot measure.
func UnavailableWorkItemMeasures() []UnavailableMeasure {
	return []UnavailableMeasure{
		{
			Measure: "edited", Reason: "not_recorded",
			Detail: "A decision approves or refuses the proposal as asked; the engine refuses " +
				"arguments whose digest differs from the approved ones, so an approval with " +
				"changes does not exist. See revisedAfterRefusal and repeatedAfterRefusal.",
		},
		{
			Measure: "laneTime", Reason: "board_only",
			Detail: "Time in each board lane is read from the project board by the factory CLI (#201, PR 3).",
		},
		{
			Measure: "caughtLater", Reason: "board_only",
			Detail: "A card sent back after it passed a lane is read from the project board by the factory CLI (#201, PR 3).",
		},
	}
}
