package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

const since = "2026-10-05T00:00:00Z"

func testReader(t *testing.T, f *fakeGitHub) reader {
	t.Helper()
	env := f.env()
	c, err := newClient(func(k string) string { return env[k] }, f.srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	return reader{c: c, owner: fakeOwner, name: fakeName, project: project, bots: testBots(t)}
}

func read(t *testing.T, f *fakeGitHub) Snapshot {
	t.Helper()
	s, err := testReader(t, f).snapshot(context.Background(), at(t, since))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func countKind(it *Item, k Kind) int {
	n := 0
	for _, e := range it.Events {
		if e.Kind == k {
			n++
		}
	}
	return n
}

func TestSnapshot_timelineOver100Items_readWhole(t *testing.T) {
	t.Parallel()
	f := newFakeGitHub(t)
	long := f.add(1, &fakeThing{onBoard: true})
	lanes := []string{"Triagem", "Preparado"}
	for i := range 250 {
		when := at(t, since).Add(time.Duration(i) * time.Minute).Format(time.RFC3339)
		long.nodes = append(long.nodes, nStatus(when, lanes[i%2], lanes[(i+1)%2], person))
	}
	for n := 2; n <= 45; n++ { // more cards than one batch holds
		f.add(n, &fakeThing{onBoard: true, nodes: []map[string]any{nComment(since, "hi", person)}})
	}
	s := read(t, f)
	if got := countKind(s.Items[1], KindStatus); got != 250 {
		t.Fatalf("read %d status events of 250", got)
	}
	for n := 2; n <= 45; n++ {
		if s.Items[n] == nil || countKind(s.Items[n], KindComment) != 1 {
			t.Fatalf("card #%d not read whole", n)
		}
	}
}

func TestSnapshot_linkedItems_followedAndOtherRepositoriesIgnored(t *testing.T) {
	t.Parallel()
	f := newFakeGitHub(t)
	old := at(t, "2026-09-01T00:00:00Z")
	f.add(10, &fakeThing{onBoard: true, linked: []int{11}})
	f.add(11, &fakeThing{isPR: true, updatedAt: old, linked: []int{10}})
	f.add(12, &fakeThing{isPR: true, updatedAt: at(t, "2026-10-06T00:00:00Z"), linked: []int{13}})
	f.add(13, &fakeThing{})
	f.add(14, &fakeThing{isPR: true, updatedAt: old}) // neither recent nor linked
	s := read(t, f)
	for _, n := range []int{10, 11, 12, 13} {
		if s.Items[n] == nil {
			t.Errorf("#%d was not read", n)
		}
	}
	if s.Items[14] != nil || s.Items[9999] != nil {
		t.Errorf("read an item no rule asked for: %v", s.Items)
	}
	if !s.Items[10].OnBoard || s.Items[13].OnBoard || !s.Items[11].IsPR || s.Items[13].IsPR {
		t.Errorf("item kinds lost: %+v %+v", s.Items[10], s.Items[13])
	}
}

func TestSnapshot_qaStatuses_readAcrossPagesOnFactoryPullRequestsOnly(t *testing.T) {
	t.Parallel()
	f := newFakeGitHub(t)
	f.restPage = 2
	recent := at(t, "2026-10-06T00:00:00Z")
	f.add(20, &fakeThing{isPR: true, updatedAt: recent, author: ghActor(devBot).(map[string]any),
		nodes: []map[string]any{nCommit(since, shaA, devBot), nReview(since, "APPROVED", shaB, revBot)}})
	f.add(21, &fakeThing{isPR: true, updatedAt: recent, nodes: []map[string]any{nCommit(since, shaC, person)}})
	f.statuses[shaA] = []map[string]any{
		rStatus("2026-10-06T03:00:00Z", "success", qaContext, qaBot),
		rStatus("2026-10-06T02:00:00Z", "success", "ci/build", strayBot),
		rStatus("2026-10-06T01:00:00Z", "failure", qaContext, qaBot),
	}
	f.statuses[shaB] = []map[string]any{rStatus("2026-10-06T04:00:00Z", "failure", qaContext, qaBot)}
	s := read(t, f)
	got := []string{}
	for _, st := range s.Items[20].Statuses {
		got = append(got, fmt.Sprintf("%.1s %s %s", st.SHA, st.State, st.Creator.Login))
	}
	if want := "a success q-app[bot]|a failure q-app[bot]|b failure q-app[bot]"; strings.Join(got, "|") != want {
		t.Errorf("statuses = %q, want %q", strings.Join(got, "|"), want)
	}
	for _, r := range f.requests {
		if strings.Contains(r.Path, shaC) {
			t.Errorf("read statuses of a pull request no role touched: %s", r.Path)
		}
	}
}

func TestSnapshot_onlyReads(t *testing.T) {
	t.Parallel()
	f := newFakeGitHub(t)
	f.add(20, &fakeThing{isPR: true, onBoard: true, updatedAt: at(t, "2026-10-06T00:00:00Z"),
		author: ghActor(devBot).(map[string]any), nodes: []map[string]any{nCommit(since, shaA, devBot)}})
	read(t, f)
	if len(f.requests) == 0 {
		t.Fatal("no request reached the fake")
	}
	for _, r := range f.requests {
		query := r.Method == http.MethodPost && r.Path == "/graphql" && strings.HasPrefix(strings.TrimSpace(r.Body), "query")
		if !query && r.Method != http.MethodGet {
			t.Errorf("a request that is not a read: %s %s %.40q", r.Method, r.Path, r.Body)
		}
	}
}

func TestSnapshot_graphQLError_failsTheRead(t *testing.T) {
	t.Parallel()
	f := newFakeGitHub(t)
	f.add(1, &fakeThing{onBoard: true})
	f.failTypes = "FORBIDDEN"
	_, err := testReader(t, f).snapshot(context.Background(), at(t, since))
	if err == nil || !strings.Contains(err.Error(), "FORBIDDEN") {
		t.Fatalf("err = %v, want the GraphQL error type", err)
	}
}

func TestClient_refusesAnythingButAQuery(t *testing.T) {
	t.Parallel()
	f := newFakeGitHub(t)
	err := testReader(t, f).c.query(context.Background(), "mutation { x }", nil, nil)
	if err == nil || len(f.requests) != 0 {
		t.Fatalf("err = %v after %d requests, want a refusal before any request", err, len(f.requests))
	}
}

// A dismissal is read with the review it dismissed, so an approval QA failed
// before it was taken away still counts as an approval at that instant.
func TestSnapshot_reviewDismissal_readWithTheReviewItDismissed(t *testing.T) {
	t.Parallel()
	f := newFakeGitHub(t)
	f.add(20, &fakeThing{isPR: true, updatedAt: at(t, "2026-10-06T00:00:00Z"), author: ghActor(devBot).(map[string]any),
		nodes: []map[string]any{
			nReviewID("2026-10-05T10:00:00Z", "DISMISSED", shaA, "PRR_1", revBot),
			nDismissed("2026-10-05T13:00:00Z", "PRR_1", "APPROVED", person),
		}})
	f.statuses[shaA] = []map[string]any{rStatus("2026-10-05T12:00:00Z", "failure", qaContext, qaBot)}
	got := signalsOf(t, read(t, f))
	if want := "reviewer: " + sigQAOnApproved; strings.Join(got, "|") != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
