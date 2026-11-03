package main

import (
	"slices"
	"time"
)

// decisions are the Reviewer states that decide something (factory/LANES.md
// § factory:ready-to-merge → Done); COMMENTED decides nothing.
func decisions() []string { return []string{"APPROVED", "CHANGES_REQUESTED", "DISMISSED"} }

// afterApproval: after the Reviewer approved, a person requested changes or
// pushed a commit, before the Reviewer's next decision. One signal per
// approval, at the first such act.
func (v prView) afterApproval() []Signal {
	var out []Signal
	for i, r := range v.reviews {
		if r.State != "APPROVED" {
			continue
		}
		until := v.nextDecision(i)
		if a, ok := firstAct(v.person, r.At, until); ok {
			out = append(out, Signal{OwnerReviewer, sigPersonAfterApproval + a.how, a.at, v.ref(r.SHA)})
		}
	}
	return out
}

func (v prView) nextDecision(i int) time.Time {
	for _, r := range v.reviews[i+1:] {
		if slices.Contains(decisions(), r.State) {
			return r.At
		}
	}
	return time.Time{}
}

// afterQAPassed: after QA recorded success, before its next status on the
// pull request, a person requested changes or pushed, the pull request was
// closed without a merge, or a person reopened an issue it closes. One
// signal per success, at the first such act.
func (v prView) afterQAPassed(s Snapshot, bots Bots) []Signal {
	acts := slices.Clone(v.person)
	for _, e := range v.pr.Events {
		if e.Kind == KindClosed && !v.pr.Merged {
			acts = append(acts, act{e.At, "closed unmerged"})
		}
	}
	for _, n := range v.pr.Linked {
		if issue := s.Items[n]; issue != nil {
			acts = append(acts, reopenedByPerson(issue, bots)...)
		}
	}
	var out []Signal
	for i, st := range v.qa {
		if st.State != "success" {
			continue
		}
		var until time.Time
		if i+1 < len(v.qa) {
			until = v.qa[i+1].At
		}
		if a, ok := firstAct(acts, st.At, until); ok {
			out = append(out, Signal{OwnerQA, sigPersonAfterQA + a.how, a.at, v.ref(st.SHA)})
		}
	}
	return out
}

func reopenedByPerson(issue *Item, bots Bots) []act {
	var out []act
	for _, e := range issue.Events {
		if e.Kind == KindReopened && bots.classify(e.Actor) == OwnerPerson {
			out = append(out, act{e.At, "issue reopened"})
		}
	}
	return out
}

// firstAct is the earliest act strictly after from and, unless until is
// zero, strictly before until.
func firstAct(acts []act, from, until time.Time) (act, bool) {
	var best act
	found := false
	for _, a := range acts {
		inside := a.at.After(from) && (until.IsZero() || a.at.Before(until))
		if inside && (!found || a.at.Before(best.at)) {
			best, found = a, true
		}
	}
	return best, found
}
