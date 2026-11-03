package main

import "time"

// Labels that decide who owns a stretch of a stay (factory/LANES.md).
const (
	labelNeedsDesign      = "factory:needs-design"
	labelNeedsReview      = "factory:needs-review"
	labelNeedsQA          = "factory:needs-qa"
	labelChangesRequested = "factory:changes-requested"
	labelReadyToMerge     = "factory:ready-to-merge"
)

// subStateOwner is who a Doing sub-state label waits on.
func subStateOwner(label string) (Owner, bool) {
	switch label {
	case labelNeedsReview:
		return OwnerReviewer, true
	case labelNeedsQA:
		return OwnerQA, true
	case labelChangesRequested:
		return OwnerDeveloper, true
	case labelReadyToMerge:
		return OwnerPerson, true
	}
	return "", false
}

// ownerAt is who had to move the card at t, by its lane:
//
//   - Triagem: the Architect while the issue carries factory:needs-design,
//     Triage otherwise.
//   - Doing: the role the pull request's sub-state label names; a person for
//     factory:ready-to-merge. With several sub-state labels present, the one
//     applied last decides. With none, nobody: unattributed.
//   - Preparado and Travado: a person; no role moves a card out of them.
func ownerAt(s Snapshot, card *Item, lane string, t time.Time) Owner {
	switch lane {
	case LaneTriagem:
		if _, on := labelsAt(card, t)[labelNeedsDesign]; on {
			return OwnerArchitect
		}
		return OwnerTriage
	case LaneDoing:
		return doingOwner(s, card, t)
	case LanePreparado, LaneTravado:
		return OwnerPerson
	}
	return OwnerUnattributed
}

func doingOwner(s Snapshot, card *Item, t time.Time) Owner {
	prs := []*Item{card}
	if !card.IsPR {
		prs = linkedPRs(s, card)
	}
	owner, latest := OwnerUnattributed, labelMark{}
	for _, pr := range prs {
		for name, mark := range labelsAt(pr, t) {
			o, ok := subStateOwner(name)
			if ok && latest.before(mark) {
				owner, latest = o, mark
			}
		}
	}
	return owner
}

// labelMark is when a label was applied: its time, then its place in the
// timeline, so two labels applied in the same second still have an order.
type labelMark struct {
	at  time.Time
	seq int
}

func (m labelMark) before(o labelMark) bool {
	return m.at.Before(o.at) || (m.at.Equal(o.at) && m.seq < o.seq)
}

// labelsAt is the labels on it at t, each with when it was last applied.
// Events at t itself count: a label added at the instant a segment starts
// owns that segment.
func labelsAt(it *Item, t time.Time) map[string]labelMark {
	on := map[string]labelMark{}
	for i, e := range it.Events {
		switch {
		case e.At.After(t):
		case e.Kind == KindLabeled:
			on[e.Label] = labelMark{e.At, i + 1}
		case e.Kind == KindUnlabeled:
			delete(on, e.Label)
		}
	}
	return on
}

// linkedPRs is the pull requests tied to an issue card: those its
// closedByPullRequestsReferences names and those whose closing references
// name it.
func linkedPRs(s Snapshot, card *Item) []*Item {
	seen := map[int]bool{}
	var out []*Item
	add := func(n int) {
		if pr := s.Items[n]; pr != nil && pr.IsPR && !seen[n] {
			seen[n] = true
			out = append(out, pr)
		}
	}
	for _, n := range card.Linked {
		add(n)
	}
	for _, n := range sortedNumbers(s) {
		if contains(s.Items[n].Linked, card.Number) {
			add(n)
		}
	}
	return out
}

func contains(ns []int, n int) bool {
	for _, x := range ns {
		if x == n {
			return true
		}
	}
	return false
}
