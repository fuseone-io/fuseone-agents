package main

import (
	"fmt"
	"slices"
	"sort"
	"time"
)

// qaContext is the commit status QA records (factory/REVIEW.md).
const qaContext = "factory/qa"

// Caught-later signals. Each names who caught what the role passed; none of
// them says the role was wrong, only that a later stage or a person acted
// against what it let through.
const (
	sigReviewerChanges     = "caught by the Reviewer: changes requested"
	sigQAOnUnreviewed      = "caught by QA on a head the Reviewer never reviewed"
	sigQAOnApproved        = "caught by QA on a head the Reviewer approved"
	sigPersonAfterApproval = "caught by a person after the Reviewer approved: "
	sigPersonAfterQA       = "caught by a person after QA passed: "
)

// Signal is one caught-later signal, counted in the week of At: when the
// catching act happened.
type Signal struct {
	Owner Owner
	Kind  string
	At    time.Time
	Ref   string // #<number>, and the head when one decides it
}

// caughtLater reads every signal the snapshot holds, in time order.
func caughtLater(s Snapshot, bots Bots, project int) []Signal {
	var out []Signal
	for _, n := range sortedNumbers(s) {
		it := s.Items[n]
		if it.IsPR {
			out = append(out, prSignals(s, it, bots)...)
		}
		if it.OnBoard {
			out = append(out, returns(s, it, bots, project)...)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].At.Equal(out[j].At) {
			return out[i].At.Before(out[j].At)
		}
		return slices.Index(owners(), out[i].Owner) < slices.Index(owners(), out[j].Owner)
	})
	return out
}

func prSignals(s Snapshot, pr *Item, bots Bots) []Signal {
	v := newPRView(pr, bots)
	out := v.reviewerCaughtDeveloper()
	out = append(out, v.qaFailures()...)
	out = append(out, v.afterApproval()...)
	return append(out, v.afterQAPassed(s, bots)...)
}

// act is something a person did on a pull request that undoes a pass.
type act struct {
	at  time.Time
	how string
}

// prView is a pull request's events sorted out by who did them.
type prView struct {
	pr      *Item
	devPR   bool
	reviews []Event  // the Reviewer bot's, in time order
	qa      []Status // factory/qa statuses the QA bot created, in time order
	person  []act    // a person's change requests and commits
}

func newPRView(pr *Item, bots Bots) prView {
	v := prView{pr: pr, devPR: bots.classify(pr.Author) == OwnerDeveloper}
	dismissed := dismissals(pr.Events)
	for _, e := range sortedEvents(pr.Events) {
		who := bots.classify(e.Actor)
		switch {
		case e.Kind == KindReview && who == OwnerReviewer:
			v.reviews = append(v.reviews, asDecided(e, dismissed)...)
		case e.Kind == KindReview && who == OwnerPerson && e.State == "CHANGES_REQUESTED":
			v.person = append(v.person, act{e.At, "changes requested"})
		case e.Kind == KindCommit && who == OwnerPerson:
			v.person = append(v.person, act{e.At, "a commit"})
		}
	}
	for _, st := range pr.Statuses {
		if st.Context == qaContext && bots.classify(st.Creator) == OwnerQA {
			v.qa = append(v.qa, st)
		}
	}
	sort.SliceStable(v.qa, func(i, j int) bool { return v.qa[i].At.Before(v.qa[j].At) })
	v.reviews = sortedEvents(v.reviews)
	return v
}

// dismissals maps a review's id to the event that dismissed it.
func dismissals(es []Event) map[string]Event {
	out := map[string]Event{}
	for _, e := range es {
		if e.Kind == KindDismissed && e.ReviewID != "" {
			out[e.ReviewID] = e
		}
	}
	return out
}

// asDecided is a review as the decisions it stood for over time. GitHub
// rewrites a dismissed review's state to DISMISSED and keeps its submittedAt,
// so on its own it reads as dismissed from the start. With the
// ReviewDismissedEvent that names it, it is its earlier state at submittedAt
// and DISMISSED from the dismissal on. Without one, it stays DISMISSED from
// submittedAt: what it said before is unknown, so it vouches for nothing.
func asDecided(r Event, dismissed map[string]Event) []Event {
	d, ok := dismissed[r.ReviewID]
	if r.State != "DISMISSED" || !ok || d.State == "" {
		return []Event{r}
	}
	before, after := r, r
	before.State = d.State
	after.At = d.At
	return []Event{before, after}
}

// standingAt is the Reviewer bot's latest decision (APPROVED,
// CHANGES_REQUESTED or DISMISSED) on sha submitted before t, and whether it
// left any review on sha before t.
func (v prView) standingAt(sha string, t time.Time) (latest string, reviewed bool) {
	for _, r := range v.reviews {
		if r.SHA != sha || !r.At.Before(t) {
			continue
		}
		reviewed = true
		if slices.Contains(decisions(), r.State) {
			latest = r.State
		}
	}
	return latest, reviewed
}

func sortedEvents(es []Event) []Event {
	out := slices.Clone(es)
	sort.SliceStable(out, func(i, j int) bool { return out[i].At.Before(out[j].At) })
	return out
}

func (v prView) ref(sha string) string {
	if sha == "" {
		return fmt.Sprintf("#%d", v.pr.Number)
	}
	return fmt.Sprintf("#%d@%.7s", v.pr.Number, sha)
}

// reviewerCaughtDeveloper: the Reviewer requested changes on a Developer
// pull request, once per reviewed commit.
func (v prView) reviewerCaughtDeveloper() []Signal {
	if !v.devPR {
		return nil
	}
	var out []Signal
	seen := map[string]bool{}
	for _, r := range v.reviews {
		if r.State == "CHANGES_REQUESTED" && !seen[r.SHA] {
			seen[r.SHA] = true
			out = append(out, Signal{OwnerDeveloper, sigReviewerChanges, r.At, v.ref(r.SHA)})
		}
	}
	return out
}

// qaFailures: QA failed a head. The Reviewer's standing on that head is
// judged at the status's instant, by its latest decision on the head
// submitted before it:
//
//   - APPROVED: the Reviewer let it through, and the signal is the Reviewer's;
//   - CHANGES_REQUESTED or DISMISSED: the head had been sent back or its
//     approval taken away, so nobody gets a signal;
//   - no review of the head at all before it, on a Developer pull request:
//     the signal is the Developer's;
//   - only COMMENTED reviews before it: they decide nothing, and the head was
//     looked at, so nobody gets a signal.
//
// Once per head, judged at its first failure.
func (v prView) qaFailures() []Signal {
	var out []Signal
	seen := map[string]bool{}
	for _, st := range v.qa {
		if st.State != "failure" || seen[st.SHA] {
			continue
		}
		seen[st.SHA] = true
		latest, reviewed := v.standingAt(st.SHA, st.At)
		switch {
		case latest == "APPROVED":
			out = append(out, Signal{OwnerReviewer, sigQAOnApproved, st.At, v.ref(st.SHA)})
		case v.devPR && !reviewed:
			out = append(out, Signal{OwnerDeveloper, sigQAOnUnreviewed, st.At, v.ref(st.SHA)})
		}
	}
	return out
}
