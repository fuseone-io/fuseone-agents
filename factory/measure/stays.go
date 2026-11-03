package main

import (
	"slices"
	"sort"
	"time"
)

// The board's lanes (factory/LANES.md). Done is where a card stops; time in
// it is not a stay.
const (
	LaneTriagem   = "Triagem"
	LanePreparado = "Preparado"
	LaneDoing     = "Doing"
	LaneTravado   = "Travado"
	LaneDone      = "Done"
)

// Segment is a stretch of a stay that one owner had to move.
type Segment struct {
	Owner Owner
	Dur   time.Duration
}

// Stay is a card's time in one lane: from the move that put it there to the
// next move. An open stay has no next move yet; it carries no segments and
// is never in a percentile.
type Stay struct {
	Card       int
	Lane       string
	Start, End time.Time
	Open       bool
	Segments   []Segment
}

// laneStays reads every board card's stays from its status-change events in
// the given project, oldest first. Events of other projects are ignored.
func laneStays(s Snapshot, project int) []Stay {
	var out []Stay
	for _, n := range sortedNumbers(s) {
		card := s.Items[n]
		if card.OnBoard {
			out = append(out, cardStays(s, card, project)...)
		}
	}
	return out
}

func sortedNumbers(s Snapshot) []int {
	var ns []int
	for n := range s.Items {
		ns = append(ns, n)
	}
	sort.Ints(ns)
	return ns
}

func statusMoves(it *Item, project int) []Event {
	var out []Event
	for _, e := range it.Events {
		if e.Kind == KindStatus && e.Project == project {
			out = append(out, e)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].At.Before(out[j].At) })
	return out
}

func cardStays(s Snapshot, card *Item, project int) []Stay {
	moves := statusMoves(card, project)
	var out []Stay
	for i, m := range moves {
		if m.To == "" || m.To == LaneDone {
			continue
		}
		st := Stay{Card: card.Number, Lane: m.To, Start: m.At, Open: i == len(moves)-1}
		if !st.Open {
			st.End = moves[i+1].At
			st.Segments = segments(s, card, st)
		}
		out = append(out, st)
	}
	return out
}

// segments splits a closed stay wherever a label that decides its owner
// changed, and gives each piece to the owner at its start.
func segments(s Snapshot, card *Item, st Stay) []Segment {
	cuts := []time.Time{st.Start, st.End}
	for _, it := range append([]*Item{card}, linkedPRs(s, card)...) {
		for _, e := range it.Events {
			if isLabelEvent(e) && e.At.After(st.Start) && e.At.Before(st.End) {
				cuts = append(cuts, e.At)
			}
		}
	}
	slices.SortFunc(cuts, func(a, b time.Time) int { return a.Compare(b) })
	cuts = slices.CompactFunc(cuts, func(a, b time.Time) bool { return a.Equal(b) })
	var out []Segment
	for i := 0; i+1 < len(cuts); i++ {
		owner := ownerAt(s, card, st.Lane, cuts[i])
		d := cuts[i+1].Sub(cuts[i])
		if n := len(out); n > 0 && out[n-1].Owner == owner {
			out[n-1].Dur += d
			continue
		}
		out = append(out, Segment{owner, d})
	}
	return out
}

func isLabelEvent(e Event) bool { return e.Kind == KindLabeled || e.Kind == KindUnlabeled }
