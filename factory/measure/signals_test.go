package main

import (
	"reflect"
	"testing"
)

const (
	shaA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	shaB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	shaC = "cccccccccccccccccccccccccccccccccccccccc"
)

func review(t *testing.T, when, state, sha string, who Actor) Event {
	return Event{Kind: KindReview, At: at(t, when), Actor: who, State: state, SHA: sha}
}

func commit(t *testing.T, when, sha string, who Actor) Event {
	return Event{Kind: KindCommit, At: at(t, when), Actor: who, SHA: sha}
}

func qaStatus(t *testing.T, when, sha, state string, who Actor) Status {
	return Status{SHA: sha, Context: qaContext, State: state, At: at(t, when), Creator: who}
}

func comment(t *testing.T, when, returnTo string, who Actor) Event {
	return Event{Kind: KindComment, At: at(t, when), Actor: who, ReturnTo: returnTo}
}

// signalsOf is each signal as "owner: kind", in time order.
func signalsOf(t *testing.T, s Snapshot) []string {
	t.Helper()
	var out []string
	for _, sig := range caughtLater(s, testBots(t), project) {
		out = append(out, string(sig.Owner)+": "+sig.Kind)
	}
	return out
}

func devPR(events []Event, statuses ...Status) *Item {
	return &Item{Number: 20, IsPR: true, Author: devBot, Events: events, Statuses: statuses}
}

func TestCaughtLater_qaFailure_goesToTheReviewerOnlyWhenItApprovedThatHead(t *testing.T) {
	t.Parallel()
	approved := devPR([]Event{review(t, "2026-10-05T10:00:00Z", "APPROVED", shaA, revBot)},
		qaStatus(t, "2026-10-05T11:00:00Z", shaA, "failure", qaBot))
	if got, want := signalsOf(t, snapshot(approved)), []string{"reviewer: " + sigQAOnApproved}; !reflect.DeepEqual(got, want) {
		t.Errorf("approved head: got %q, want %q", got, want)
	}
	unreviewed := devPR([]Event{review(t, "2026-10-05T10:00:00Z", "APPROVED", shaA, revBot)},
		qaStatus(t, "2026-10-05T11:00:00Z", shaB, "failure", qaBot),
		qaStatus(t, "2026-10-05T12:00:00Z", shaB, "failure", qaBot))
	if got, want := signalsOf(t, snapshot(unreviewed)), []string{"developer: " + sigQAOnUnreviewed}; !reflect.DeepEqual(got, want) {
		t.Errorf("never-reviewed head: got %q, want %q", got, want)
	}
}

func dismissal(t *testing.T, when, reviewID, previous string, who Actor) Event {
	return Event{Kind: KindDismissed, At: at(t, when), Actor: who, ReviewID: reviewID, State: previous}
}

func reviewWithID(t *testing.T, when, state, sha, id string, who Actor) Event {
	e := review(t, when, state, sha, who)
	e.ReviewID = id
	return e
}

// The Reviewer's standing on a head is judged at the instant QA failed it:
// its latest decision on that head submitted before the status.
func TestCaughtLater_qaFailure_judgedByTheReviewersLatestDecisionBeforeIt(t *testing.T) {
	t.Parallel()
	fail := qaStatus(t, "2026-10-05T12:00:00Z", shaA, "failure", qaBot)
	for _, tc := range []struct {
		name    string
		reviews []Event
		want    []string
	}{
		{"approved, then the failure", []Event{
			review(t, "2026-10-05T10:00:00Z", "APPROVED", shaA, revBot),
		}, []string{"reviewer: " + sigQAOnApproved}},
		{"an approval after the failure", []Event{
			review(t, "2026-10-05T13:00:00Z", "APPROVED", shaA, revBot),
		}, []string{"developer: " + sigQAOnUnreviewed}},
		{"approved, changes requested, then the failure", []Event{
			review(t, "2026-10-05T10:00:00Z", "APPROVED", shaA, revBot),
			review(t, "2026-10-05T11:00:00Z", "CHANGES_REQUESTED", shaA, revBot),
		}, []string{"developer: " + sigReviewerChanges}}, // the change request's own signal; the failure gives none
		{"approved, dismissed, then the failure", []Event{
			reviewWithID(t, "2026-10-05T10:00:00Z", "DISMISSED", shaA, "R1", revBot),
			dismissal(t, "2026-10-05T11:00:00Z", "R1", "APPROVED", person),
		}, nil},
		{"approved, the failure, dismissed after it", []Event{
			reviewWithID(t, "2026-10-05T10:00:00Z", "DISMISSED", shaA, "R1", revBot),
			dismissal(t, "2026-10-05T13:00:00Z", "R1", "APPROVED", person),
		}, []string{"reviewer: " + sigQAOnApproved}},
		{"a dismissal the timeline does not explain", []Event{
			review(t, "2026-10-05T10:00:00Z", "DISMISSED", shaA, revBot),
		}, nil},
		{"changes requested, then approved, then the failure", []Event{
			review(t, "2026-10-05T09:00:00Z", "CHANGES_REQUESTED", shaA, revBot),
			review(t, "2026-10-05T10:00:00Z", "APPROVED", shaA, revBot),
		}, []string{"developer: " + sigReviewerChanges, "reviewer: " + sigQAOnApproved}},
	} {
		got := signalsOf(t, snapshot(devPR(tc.reviews, fail)))
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestCaughtLater_qaFailureNotByTheQABot_isNoSignal(t *testing.T) {
	t.Parallel()
	pr := devPR(nil,
		qaStatus(t, "2026-10-05T11:00:00Z", shaA, "failure", person),
		qaStatus(t, "2026-10-05T11:00:00Z", shaA, "failure", strayBot),
		qaStatus(t, "2026-10-05T11:00:00Z", shaA, "failure", revBot))
	if got := signalsOf(t, snapshot(pr)); len(got) != 0 {
		t.Fatalf("got %q, want none", got)
	}
}

func TestCaughtLater_reviewerChangesRequested_oncePerReviewedCommit(t *testing.T) {
	t.Parallel()
	pr := devPR([]Event{
		review(t, "2026-10-05T10:00:00Z", "CHANGES_REQUESTED", shaA, revBot),
		review(t, "2026-10-05T10:05:00Z", "CHANGES_REQUESTED", shaA, revBot),
		review(t, "2026-10-05T12:00:00Z", "CHANGES_REQUESTED", shaB, revBot),
		review(t, "2026-10-05T13:00:00Z", "CHANGES_REQUESTED", shaC, person),
	})
	want := []string{"developer: " + sigReviewerChanges, "developer: " + sigReviewerChanges}
	if got := signalsOf(t, snapshot(pr)); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
	notDev := devPR(pr.Events)
	notDev.Author = person
	if got := signalsOf(t, snapshot(notDev)); len(got) != 0 {
		t.Fatalf("a person's pull request: got %q, want none", got)
	}
}

func TestCaughtLater_personAfterReviewerApproval_isTheReviewers(t *testing.T) {
	t.Parallel()
	pr := devPR([]Event{
		review(t, "2026-10-05T10:00:00Z", "APPROVED", shaA, revBot),
		commit(t, "2026-10-05T11:00:00Z", shaB, person),
		review(t, "2026-10-05T12:00:00Z", "CHANGES_REQUESTED", shaB, person),
		review(t, "2026-10-06T10:00:00Z", "APPROVED", shaB, revBot),
		review(t, "2026-10-06T11:00:00Z", "CHANGES_REQUESTED", shaB, person),
		review(t, "2026-10-07T10:00:00Z", "APPROVED", shaC, revBot),
		commit(t, "2026-10-07T11:00:00Z", shaC, devBot),
	})
	want := []string{
		"reviewer: " + sigPersonAfterApproval + "a commit",
		"reviewer: " + sigPersonAfterApproval + "changes requested",
	}
	if got := signalsOf(t, snapshot(pr)); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestCaughtLater_personAfterQAPassed_isQAs(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		after  []Event
		merged bool
		issue  []Event
		want   string
	}{
		{"a push", []Event{commit(t, "2026-10-06T10:00:00Z", shaB, person)}, true, nil, "a commit"},
		{"changes requested", []Event{review(t, "2026-10-06T10:00:00Z", "CHANGES_REQUESTED", shaA, person)}, true, nil, "changes requested"},
		{"closed unmerged", []Event{{Kind: KindClosed, At: at(t, "2026-10-06T10:00:00Z"), Actor: person}}, false, nil, "closed unmerged"},
		{"issue reopened", nil, true, []Event{{Kind: KindReopened, At: at(t, "2026-10-08T10:00:00Z"), Actor: person}}, "issue reopened"},
		{"merged and left alone", []Event{{Kind: KindClosed, At: at(t, "2026-10-06T10:00:00Z"), Actor: person}}, true, nil, ""},
		{"reopened by a bot", nil, true, []Event{{Kind: KindReopened, At: at(t, "2026-10-08T10:00:00Z"), Actor: strayBot}}, ""},
	} {
		pr := devPR(append([]Event{review(t, "2026-10-05T09:00:00Z", "APPROVED", shaA, revBot)}, tc.after...),
			qaStatus(t, "2026-10-05T10:00:00Z", shaA, "success", qaBot))
		pr.Merged, pr.Linked = tc.merged, []int{30}
		issue := &Item{Number: 30, Linked: []int{20}, Events: tc.issue}
		var want []string
		if tc.want != "" {
			want = []string{"qa: " + sigPersonAfterQA + tc.want}
		}
		got := signalsOf(t, snapshot(pr, issue))
		if tc.want == "a commit" || tc.want == "changes requested" {
			got = got[1:] // the Reviewer approved the same head: its signal comes first
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s: got %q, want %q", tc.name, got, want)
		}
	}
}
