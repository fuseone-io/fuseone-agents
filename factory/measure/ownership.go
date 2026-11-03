package main

import "time"

// Labels that decide who owns a stretch of a stay (factory/LANES.md).
const (
	labelNeedsDesign      = "factory:needs-design"
	labelNeedsReview      = "factory:needs-review"
	labelNeedsQA          = "factory:needs-qa"
	labelChangesRequested = "factory:changes-requested"
	labelReadyToMerge     = "factory:ready-to-merge"
	labelNeedsRuntime     = "factory:needs-runtime"
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
		return OwnerMerger, true
	}
	return "", false
}

// ownerAt is who had to move the card at t, by its lane:
//
//   - Triagem: the Architect while the issue carries factory:needs-design,
//     Triage otherwise.
//   - Doing: the role the pull request's sub-state label names, the Merger
//     for factory:ready-to-merge. With several sub-state labels present, the
//     one applied last decides. With none, nobody: unattributed.
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

// doingOwner is who a Doing card waits on at t, the first rule that applies:
//
//   - while a merged pull request still carries factory:ready-to-merge, the
//     Merger: it finishes what list_ready_prs lists under merged before any
//     other pull request, and close_merged reads no issue label, so this
//     outranks an open pull request and factory:needs-runtime alike;
//   - while the issue carries factory:needs-runtime, a person: the Developer
//     skips it and a person takes it (factory/LANES.md);
//   - before any pull request linked to the issue was opened, the Developer;
//   - with at least one linked pull request open, the role the sub-state
//     label of the open ones names, last applied wins; with no recognised
//     sub-state label, nobody, an inconsistent state not guessed at. Labels
//     on closed or merged pull requests count for nothing;
//   - after an attempt, while every linked pull request is closed or merged,
//     a person: a pull request closed without a merge leaves the next attempt
//     to a person, and a merged one close_merged finished leaves the card to
//     a person too.
//
// A pull request card exists from the start of its stay; while it is open its
// own label decides, and once it is closed or merged the time is a person's.
func doingOwner(s Snapshot, card *Item, t time.Time) Owner {
	prs := []*Item{card}
	if !card.IsPR {
		prs = openedBy(linkedPRs(s, card), t)
	}
	if mergedStillReady(prs, t) {
		return OwnerMerger
	}
	if !card.IsPR {
		if _, on := labelsAt(card, t)[labelNeedsRuntime]; on {
			return OwnerPerson
		}
		if len(prs) == 0 {
			return OwnerDeveloper
		}
	}
	open := openAt(prs, t)
	if len(open) == 0 {
		return OwnerPerson
	}
	return subStateAt(open, t)
}

// openAt is the pull requests open at t: not closed or merged at t, or
// reopened since their last close. Events at t itself count.
func openAt(prs []*Item, t time.Time) []*Item {
	var out []*Item
	for _, pr := range prs {
		open := true
		for _, e := range sortedEvents(pr.Events) {
			switch {
			case e.At.After(t):
			case e.Kind == KindClosed || e.Kind == KindMerged:
				open = false
			case e.Kind == KindReopened:
				open = true
			}
		}
		if open {
			out = append(out, pr)
		}
	}
	return out
}

// mergedStillReady says whether a pull request merged by t still carries
// factory:ready-to-merge at t: a merge close_merged has not finished.
func mergedStillReady(prs []*Item, t time.Time) bool {
	for _, pr := range prs {
		if _, on := labelsAt(pr, t)[labelReadyToMerge]; on && mergedBy(pr, t) {
			return true
		}
	}
	return false
}

func mergedBy(pr *Item, t time.Time) bool {
	for _, e := range pr.Events {
		if e.Kind == KindMerged && !e.At.After(t) {
			return true
		}
	}
	return false
}

// changesHands says whether an event can change who owns Doing time: a label
// change, or a pull request closed, merged or reopened.
func changesHands(e Event) bool {
	switch e.Kind {
	case KindLabeled, KindUnlabeled, KindClosed, KindMerged, KindReopened:
		return true
	}
	return false
}

// openedBy is the pull requests already opened at t. One whose opening time
// was not read counts as opened: the Developer is never credited with time a
// pull request may already have existed in.
func openedBy(prs []*Item, t time.Time) []*Item {
	var out []*Item
	for _, pr := range prs {
		if !pr.Created.After(t) {
			out = append(out, pr)
		}
	}
	return out
}

// subStateAt is the owner the sub-state label applied last names, across
// prs, at t; unattributed when none carries one.
func subStateAt(prs []*Item, t time.Time) Owner {
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
