package simulate_test

import (
	"context"
	"sort"
	"sync"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/simulate"
)

func casesOf(ids ...string) []simulate.Occurrence {
	out := make([]simulate.Occurrence, 0, len(ids))
	for _, id := range ids {
		out = append(out, simulate.Occurrence{ID: id, Input: []byte(`{"case":"` + id + `"}`)})
	}
	return out
}

// casesRun is each case a battery's runs replay, sorted, with repeats left in.
func casesRun(t *testing.T, store *ledger.Memory, battery string) []string {
	t.Helper()
	report, err := simulate.Gather(context.Background(), store, battery)
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	out := make([]string, 0, len(report.Cases))
	for _, c := range report.Cases {
		out = append(out, c.ID)
	}
	sort.Strings(out)
	return out
}

/*
Two workers open one window, on either side of a correction.

They ask for the same battery, and the corpus they read differs by a case that sorts
first, so every position after it names another case. Keyed by position, the second
worker gets the first worker's run back for "a" and never runs it, and "c" runs twice.
*/
func TestOpen_twoWorkersWithDifferentCorpora_runEachCaseOnce(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name          string
		first, second []simulate.Occurrence
	}{
		{"a case added that sorts first", casesOf("b", "c"), casesOf("a", "b", "c")},
		{"a case deleted that sorted first", casesOf("a", "b", "c"), casesOf("b", "c")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			store := ledger.NewMemory()
			opener := openerFor(store, engine.NewMemoryContent())
			ctx := context.Background()

			for version, corpus := range [][]simulate.Occurrence{tc.first, tc.second} {
				if _, err := simulate.Open(ctx, opener, simulate.Batch{
					ID: "battery", Agent: "triage", By: "platform",
					DatasetVersion: int64(version + 1), Cases: corpus,
				}); err != nil {
					t.Fatalf("Open: %v", err)
				}
			}

			got := casesRun(t, store, "battery")
			want := []string{"a", "b", "c"}
			if len(got) != len(want) || got[0] != "a" || got[1] != "b" || got[2] != "c" {
				t.Errorf("cases run = %v, want each once: %v", got, want)
			}
		})
	}
}

// The same, at the same moment. Under -race, and for the opener's own uniqueness.
func TestOpen_twoWorkersAtOnce_runEachCaseOnce(t *testing.T) {
	t.Parallel()
	store := ledger.NewMemory()
	opener := openerFor(store, engine.NewMemoryContent())
	ctx := context.Background()

	var group sync.WaitGroup
	for version, corpus := range [][]simulate.Occurrence{casesOf("b", "c"), casesOf("a", "b", "c")} {
		group.Go(func() {
			if _, err := simulate.Open(ctx, opener, simulate.Batch{
				ID: "battery", Agent: "triage", By: "platform",
				DatasetVersion: int64(version + 1), Cases: corpus,
			}); err != nil {
				t.Errorf("Open: %v", err)
			}
		})
	}
	group.Wait()

	if got := casesRun(t, store, "battery"); len(got) != 3 || got[0] != "a" || got[1] != "b" || got[2] != "c" {
		t.Errorf("cases run = %v, want each once", got)
	}
}

// What ran is real, and which version it came from is not one thing. So it says neither.
func TestOpen_twoWorkersWithDifferentCorpora_leaveTheBatteryUnpinned(t *testing.T) {
	t.Parallel()
	store := ledger.NewMemory()
	opener := openerFor(store, engine.NewMemoryContent())
	ctx := context.Background()

	for version, corpus := range [][]simulate.Occurrence{casesOf("b"), casesOf("a", "b")} {
		if _, err := simulate.Open(ctx, opener, simulate.Batch{
			ID: "battery", Agent: "triage", By: "platform",
			DatasetVersion: int64(version + 1), Cases: corpus,
		}); err != nil {
			t.Fatalf("Open: %v", err)
		}
	}

	report, err := simulate.Gather(ctx, store, "battery")
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	if report.DatasetVersion != 0 {
		t.Errorf("dataset version = %d, want none", report.DatasetVersion)
	}
}

// A retry after a timeout reaches the runs it already opened, corpus or uploaded set.
func TestOpen_theSameBatchAgain_opensNothingNew(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		cases []simulate.Occurrence
	}{
		{"named cases", casesOf("a", "b")},
		{"an uploaded set", []simulate.Occurrence{{Input: []byte(`{"n":1}`)}, {Input: []byte(`{"n":2}`)}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			store := ledger.NewMemory()
			opener := openerFor(store, engine.NewMemoryContent())
			ctx := context.Background()
			batch := simulate.Batch{ID: "battery", Agent: "triage", By: domain.UserID("ana"), Cases: tc.cases}

			if _, err := simulate.Open(ctx, opener, batch); err != nil {
				t.Fatalf("Open: %v", err)
			}
			if _, err := simulate.Open(ctx, opener, batch); err != nil {
				t.Fatalf("Open again: %v", err)
			}

			if got := casesRun(t, store, "battery"); len(got) != 2 {
				t.Errorf("runs = %d, want 2", len(got))
			}
		})
	}
}
