package main

import (
	"reflect"
	"testing"
	"time"
)

const project = 2

var (
	person   = Actor{"User", "alice"}
	triBot   = Actor{"Bot", "t-app[bot]"}
	archBot  = Actor{"Bot", "a-app[bot]"}
	devBot   = Actor{"Bot", "d-app[bot]"}
	revBot   = Actor{"Bot", "r-app[bot]"}
	qaBot    = Actor{"Bot", "q-app[bot]"}
	strayBot = Actor{"Bot", "stray-app[bot]"}
)

func at(t *testing.T, s string) time.Time {
	t.Helper()
	v, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func move(t *testing.T, when, from, to string, who Actor) Event {
	return Event{Kind: KindStatus, At: at(t, when), Actor: who, Project: project, From: from, To: to}
}

func label(t *testing.T, when string, on bool, name string, who Actor) Event {
	k := KindUnlabeled
	if on {
		k = KindLabeled
	}
	return Event{Kind: k, At: at(t, when), Actor: who, Label: name}
}

func testBots(t *testing.T) Bots {
	t.Helper()
	b, err := parseBots([]byte(validBots))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func snapshot(items ...*Item) Snapshot {
	s := Snapshot{Items: map[int]*Item{}}
	for _, it := range items {
		s.Items[it.Number] = it
	}
	return s
}

func TestStays_statusEvents_becomeStaysEndingAtTheNextMove(t *testing.T) {
	t.Parallel()
	card := &Item{Number: 10, OnBoard: true, Events: []Event{
		move(t, "2026-10-05T08:00:00Z", "", "Triagem", person),
		{Kind: KindStatus, At: at(t, "2026-10-05T09:00:00Z"), Actor: person, Project: 7, From: "Triagem", To: "Doing"},
		move(t, "2026-10-05T10:00:00Z", "Triagem", "Preparado", triBot),
		move(t, "2026-10-06T10:00:00Z", "Preparado", "Doing", person),
		move(t, "2026-10-07T10:00:00Z", "Doing", "Done", person),
		move(t, "2026-10-08T10:00:00Z", "Done", "Travado", person),
	}}
	got := laneStays(snapshot(card), project)
	want := []string{
		"Triagem 2026-10-05T08:00:00Z..2026-10-05T10:00:00Z",
		"Preparado 2026-10-05T10:00:00Z..2026-10-06T10:00:00Z",
		"Doing 2026-10-06T10:00:00Z..2026-10-07T10:00:00Z",
		"Travado 2026-10-08T10:00:00Z..open",
	}
	if s := describeStays(got); !reflect.DeepEqual(s, want) {
		t.Fatalf("stays:\n got %q\nwant %q", s, want)
	}
}

func describeStays(st []Stay) []string {
	var out []string
	for _, s := range st {
		end := "open"
		if !s.Open {
			end = s.End.Format(time.RFC3339)
		}
		out = append(out, s.Lane+" "+s.Start.Format(time.RFC3339)+".."+end)
	}
	return out
}

func TestStays_ownership_separatesAPersonsTimeFromARolesTime(t *testing.T) {
	t.Parallel()
	issue := &Item{Number: 10, OnBoard: true, Events: []Event{
		move(t, "2026-10-05T00:00:00Z", "", "Triagem", person),
		label(t, "2026-10-05T02:00:00Z", true, "factory:needs-design", triBot),
		label(t, "2026-10-05T05:00:00Z", false, "factory:needs-design", archBot),
		move(t, "2026-10-05T06:00:00Z", "Triagem", "Preparado", archBot),
		move(t, "2026-10-05T10:00:00Z", "Preparado", "Doing", person),
		move(t, "2026-10-06T10:00:00Z", "Doing", "Travado", qaBot),
		move(t, "2026-10-06T12:00:00Z", "Travado", "Done", person),
	}}
	pr := &Item{Number: 11, IsPR: true, Linked: []int{10}, Events: []Event{
		label(t, "2026-10-05T12:00:00Z", true, "factory:needs-review", devBot),
		label(t, "2026-10-05T15:00:00Z", false, "factory:needs-review", revBot),
		label(t, "2026-10-05T15:00:00Z", true, "factory:changes-requested", revBot),
		label(t, "2026-10-05T16:00:00Z", false, "factory:changes-requested", devBot),
		label(t, "2026-10-05T16:00:00Z", true, "factory:needs-qa", devBot),
		label(t, "2026-10-06T04:00:00Z", true, "factory:ready-to-merge", qaBot),
	}}
	got := map[string][]Segment{}
	for _, s := range laneStays(snapshot(issue, pr), project) {
		got[s.Lane] = s.Segments
	}
	h := time.Hour
	want := map[string][]Segment{
		"Triagem":   {{OwnerTriage, 2 * h}, {OwnerArchitect, 3 * h}, {OwnerTriage, 1 * h}},
		"Preparado": {{OwnerPerson, 4 * h}},
		"Doing": {{OwnerUnattributed, 2 * h}, {OwnerReviewer, 3 * h}, {OwnerDeveloper, 1 * h},
			{OwnerQA, 12 * h}, {OwnerPerson, 6 * h}},
		"Travado": {{OwnerPerson, 2 * h}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("segments:\n got %v\nwant %v", got, want)
	}
}

// Doing before any linked pull request exists waits on the Developer; with
// factory:needs-runtime on the issue it waits on a person; with a pull
// request but no sub-state label it is nobody's, an inconsistent state.
func TestStays_doingOwner_byWhetherThePullRequestExistsYet(t *testing.T) {
	t.Parallel()
	h := time.Hour
	doing := func(extra ...Event) *Item {
		return &Item{Number: 10, OnBoard: true, Events: append([]Event{
			move(t, "2026-10-05T10:00:00Z", "Preparado", "Doing", person),
			move(t, "2026-10-05T20:00:00Z", "Doing", "Done", person),
		}, extra...)}
	}
	pr := func(created string, events ...Event) *Item {
		return &Item{Number: 11, IsPR: true, Linked: []int{10}, Created: at(t, created), Events: events}
	}
	for _, tc := range []struct {
		name  string
		items []*Item
		want  []Segment
	}{
		{"no pull request yet", []*Item{doing()}, []Segment{{OwnerDeveloper, 10 * h}}},
		{"the stay spans the pull request's creation", []*Item{doing(),
			pr("2026-10-05T13:00:00Z", label(t, "2026-10-05T13:00:00Z", true, labelNeedsReview, devBot))},
			[]Segment{{OwnerDeveloper, 3 * h}, {OwnerReviewer, 7 * h}}},
		{"a pull request with no sub-state", []*Item{doing(), pr("2026-10-05T13:00:00Z")},
			[]Segment{{OwnerDeveloper, 3 * h}, {OwnerUnattributed, 7 * h}}},
		{"needs-runtime on the issue", []*Item{doing(
			label(t, "2026-10-05T12:00:00Z", true, labelNeedsRuntime, devBot),
			label(t, "2026-10-05T18:00:00Z", false, labelNeedsRuntime, person))},
			[]Segment{{OwnerDeveloper, 2 * h}, {OwnerPerson, 6 * h}, {OwnerDeveloper, 2 * h}}},
		{"a pull request opened in another card's time is not this one's", []*Item{doing(),
			pr("2026-10-05T21:00:00Z", label(t, "2026-10-05T21:00:00Z", true, labelNeedsReview, devBot))},
			[]Segment{{OwnerDeveloper, 10 * h}}},
	} {
		var got []Segment
		for _, s := range laneStays(snapshot(tc.items...), project) {
			got = append(got, s.Segments...)
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s:\n got %v\nwant %v", tc.name, got, tc.want)
		}
	}
}

// Only an open pull request's sub-state owns Doing time. Once every attempt
// is closed or merged, the time is a person's: a pull request closed without
// a merge leaves the next attempt to a person (factory/LANES.md).
func TestStays_doingOwner_onlyOpenPullRequestsOwnTime(t *testing.T) {
	t.Parallel()
	h := time.Hour
	issue := &Item{Number: 10, OnBoard: true, Events: []Event{
		move(t, "2026-10-05T10:00:00Z", "Travado", "Doing", person),
		move(t, "2026-10-05T20:00:00Z", "Doing", "Done", person),
	}}
	pr := func(n int, created string, events ...Event) *Item {
		return &Item{Number: n, IsPR: true, Linked: []int{10}, Created: at(t, created), Events: events}
	}
	ev := func(k Kind, when string) Event { return Event{Kind: k, At: at(t, when), Actor: person} }
	for _, tc := range []struct {
		name string
		prs  []*Item
		want []Segment
	}{
		{"a closed attempt still labelled needs-review", []*Item{pr(11, "2026-10-05T07:00:00Z",
			label(t, "2026-10-05T07:00:00Z", true, labelNeedsReview, devBot),
			ev(KindClosed, "2026-10-05T09:00:00Z"))},
			[]Segment{{OwnerPerson, 10 * h}}},
		{"merged, then more Doing time", []*Item{pr(11, "2026-10-05T07:00:00Z",
			label(t, "2026-10-05T07:00:00Z", true, labelNeedsQA, revBot),
			ev(KindMerged, "2026-10-05T14:00:00Z"), ev(KindClosed, "2026-10-05T14:00:00Z"))},
			[]Segment{{OwnerQA, 4 * h}, {OwnerPerson, 6 * h}}},
		{"closed, then reopened", []*Item{pr(11, "2026-10-05T07:00:00Z",
			label(t, "2026-10-05T07:00:00Z", true, labelNeedsReview, devBot),
			ev(KindClosed, "2026-10-05T09:00:00Z"), ev(KindReopened, "2026-10-05T15:00:00Z"))},
			[]Segment{{OwnerPerson, 5 * h}, {OwnerReviewer, 5 * h}}},
		{"a second attempt after the first was closed", []*Item{
			pr(11, "2026-10-05T07:00:00Z", label(t, "2026-10-05T07:00:00Z", true, labelNeedsReview, devBot),
				ev(KindClosed, "2026-10-05T12:00:00Z")),
			pr(12, "2026-10-05T14:00:00Z", label(t, "2026-10-05T14:00:00Z", true, labelNeedsReview, devBot))},
			[]Segment{{OwnerReviewer, 2 * h}, {OwnerPerson, 2 * h}, {OwnerReviewer, 6 * h}}},
		{"one open, one closed with a newer stale label", []*Item{
			pr(11, "2026-10-05T07:00:00Z", label(t, "2026-10-05T07:00:00Z", true, labelNeedsQA, revBot)),
			pr(12, "2026-10-05T06:00:00Z", ev(KindClosed, "2026-10-05T08:00:00Z"),
				label(t, "2026-10-05T11:00:00Z", true, labelNeedsReview, devBot))},
			[]Segment{{OwnerQA, 10 * h}}},
	} {
		var got []Segment
		for _, s := range laneStays(snapshot(append([]*Item{issue}, tc.prs...)...), project) {
			got = append(got, s.Segments...)
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s:\n got %v\nwant %v", tc.name, got, tc.want)
		}
	}
}

func TestWeekStart_sundayLastInstant_staysInItsWeek(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ at, want string }{
		{"2026-10-11T23:59:59.999Z", "2026-10-05T00:00:00Z"},
		{"2026-10-12T00:00:00Z", "2026-10-12T00:00:00Z"},
		{"2026-10-12T01:00:00+03:00", "2026-10-05T00:00:00Z"}, // Sunday 22:00 UTC
		{"2026-10-07T12:00:00Z", "2026-10-05T00:00:00Z"},
	} {
		if got := weekStart(at(t, tc.at)); !got.Equal(at(t, tc.want)) {
			t.Errorf("weekStart(%s) = %s, want %s", tc.at, got, tc.want)
		}
	}
}

func TestPercentile_nearestRank(t *testing.T) {
	t.Parallel()
	h := time.Hour
	d := []time.Duration{1 * h, 2 * h, 3 * h, 4 * h, 5 * h, 6 * h, 7 * h, 8 * h, 9 * h, 10 * h}
	if got := percentile(d, 50); got != 5*h {
		t.Errorf("p50 = %s, want 5h", got)
	}
	if got := percentile(d, 90); got != 9*h {
		t.Errorf("p90 = %s, want 9h", got)
	}
	if got := percentile(d[:1], 90); got != 1*h {
		t.Errorf("p90 of one = %s, want 1h", got)
	}
}
