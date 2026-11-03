package main

import (
	"slices"
	"time"
)

// Scope is what one report covers and when it was read.
type Scope struct {
	Repo    string
	Project int
	Window  Window
	Now     time.Time // the injected clock: an open stay's age is measured to it
}

// Measures is a report's content, before it is printed.
type Measures struct {
	Scope
	Lanes    []laneRow
	Roles    []roleRow
	Signals  []signalRow
	Open     []openRow
	Unmapped []unmappedRow
}

// laneRow is one lane in one week: the closed stays that ended that week.
type laneRow struct {
	Week        time.Time
	Lane        string
	Stays       int
	Median, P90 time.Duration
}

// roleRow is one owner in one week: the time it owned in the closed stays
// that ended that week, the board moves it made, and the signals caught
// against it.
type roleRow struct {
	Week   time.Time
	Owner  Owner
	Owned  time.Duration
	Moves  int
	Caught int
}

type signalRow struct {
	Week  time.Time
	Owner Owner
	Kind  string
	Count int
}

// openRow is the stays still open in a lane, now: never in a percentile.
type openRow struct {
	Lane   string
	Open   int
	Oldest time.Duration
}

func measure(s Snapshot, bots Bots, sc Scope) Measures {
	stays := laneStays(s, sc.Project)
	signals := caughtLater(s, bots, sc.Project)
	return Measures{
		Scope:    sc,
		Lanes:    laneRows(stays, sc.Window),
		Roles:    roleRows(s, bots, stays, signals, sc),
		Signals:  signalRows(signals, sc.Window),
		Open:     openRows(stays, sc.Now),
		Unmapped: unmappedRows(s, bots, sc.Window),
	}
}

// laneOrder is the board's lanes as people read them, then any other lane
// name the events carry, in the order met.
func laneOrder(stays []Stay) []string {
	out := []string{LaneTriagem, LanePreparado, LaneDoing, LaneTravado}
	for _, st := range stays {
		if !slices.Contains(out, st.Lane) {
			out = append(out, st.Lane)
		}
	}
	return out
}

// laneRows: every closed stay counts in the week it ended. An open stay has
// not ended, so it is in no week.
func laneRows(stays []Stay, w Window) []laneRow {
	var out []laneRow
	for _, wk := range w.weeks() {
		for _, lane := range laneOrder(stays) {
			if d := closedIn(stays, lane, wk); len(d) > 0 {
				out = append(out, laneRow{wk, lane, len(d), percentile(d, 50), percentile(d, 90)})
			}
		}
	}
	return out
}

// closedIn is the length of every closed stay in lane that ended in wk.
func closedIn(stays []Stay, lane string, wk time.Time) []time.Duration {
	var d []time.Duration
	for _, st := range stays {
		if !st.Open && st.Lane == lane && weekStart(st.End).Equal(wk) {
			d = append(d, st.End.Sub(st.Start))
		}
	}
	return d
}

func roleRows(s Snapshot, bots Bots, stays []Stay, signals []Signal, sc Scope) []roleRow {
	var out []roleRow
	for _, wk := range sc.Window.weeks() {
		for _, o := range owners() {
			r := roleRow{Week: wk, Owner: o, Owned: ownedIn(stays, o, wk),
				Moves: movesBy(s, bots, sc.Project, o, wk), Caught: len(signalsIn(signals, o, wk))}
			if o.isRole() || r.Owned > 0 || r.Moves > 0 || r.Caught > 0 {
				out = append(out, r)
			}
		}
	}
	return out
}

// ownedIn is the time o owned in the closed stays that ended in wk.
func ownedIn(stays []Stay, o Owner, wk time.Time) time.Duration {
	var d time.Duration
	for _, st := range stays {
		if !st.Open && weekStart(st.End).Equal(wk) {
			d += ownedBy(st, o)
		}
	}
	return d
}

func signalsIn(signals []Signal, o Owner, wk time.Time) []Signal {
	var out []Signal
	for _, sig := range signals {
		if sig.Owner == o && weekStart(sig.At).Equal(wk) {
			out = append(out, sig)
		}
	}
	return out
}

func ownedBy(st Stay, o Owner) time.Duration {
	var d time.Duration
	for _, seg := range st.Segments {
		if seg.Owner == o {
			d += seg.Dur
		}
	}
	return d
}

func movesBy(s Snapshot, bots Bots, project int, o Owner, wk time.Time) int {
	n := 0
	for _, it := range s.Items {
		for _, e := range statusMoves(it, project) {
			if it.OnBoard && weekStart(e.At).Equal(wk) && bots.classify(e.Actor) == o {
				n++
			}
		}
	}
	return n
}

func signalRows(signals []Signal, w Window) []signalRow {
	var out []signalRow
	for _, wk := range w.weeks() {
		for _, o := range owners() {
			out = append(out, kindRows(signalsIn(signals, o, wk), o, wk)...)
		}
	}
	return out
}

// kindRows counts one owner's signals of one week by kind, kinds sorted.
func kindRows(signals []Signal, o Owner, wk time.Time) []signalRow {
	count := map[string]int{}
	var kinds []string
	for _, sig := range signals {
		if count[sig.Kind] == 0 {
			kinds = append(kinds, sig.Kind)
		}
		count[sig.Kind]++
	}
	slices.Sort(kinds)
	var out []signalRow
	for _, k := range kinds {
		out = append(out, signalRow{wk, o, k, count[k]})
	}
	return out
}

func openRows(stays []Stay, now time.Time) []openRow {
	var out []openRow
	for _, lane := range laneOrder(stays) {
		r := openRow{Lane: lane}
		for _, st := range stays {
			if st.Open && st.Lane == lane {
				r.Open++
				r.Oldest = max(r.Oldest, now.Sub(st.Start))
			}
		}
		if r.Open > 0 {
			out = append(out, r)
		}
	}
	return out
}
