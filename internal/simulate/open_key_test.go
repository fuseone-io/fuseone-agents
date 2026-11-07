package simulate_test

import (
	"context"
	"encoding/json"
	"sort"
	"sync"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/simulate"
	"github.com/fuseone/agents/internal/trigger"
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

func appendStep(t *testing.T, store *ledger.Memory, run domain.RunID, kind domain.StepKind, payload any) {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if _, err := store.Append(context.Background(), domain.Step{
		RunID: run, Kind: kind, AgentID: "suporte", VersionID: "v1", Payload: raw,
		Scope: domain.Scope{Company: "acme", Area: "cx"},
	}); err != nil {
		t.Fatalf("append: %v", err)
	}
}

/*
A pod of the previous release and a pod of this one open one window. Their keys differ, so
"estorno" gets two runs, through the real opener. The first reaches the tool and the second
does not, in either order, and the case must read as broken: judging only one of them would
let an agent start on a correction that just failed.
*/
func TestBattery_anOldKeyAndANewKeyWithDifferentResults_readAsBroken(t *testing.T) {
	t.Parallel()
	for _, heldFirst := range []bool{true, false} {
		store := ledger.NewMemory()
		opener := openerFor(store, engine.NewMemoryContent())
		ctx := context.Background()

		old, err := opener.Open(ctx, trigger.Request{
			Agent: "triage", IdemKey: "sim:battery:1", Trigger: "simulation", By: "platform",
			Simulation: "battery", Case: "estorno", Input: []byte(`{}`),
		})
		if err != nil {
			t.Fatalf("Open with the previous key: %v", err)
		}
		opened, err := simulate.Open(ctx, opener, simulate.Batch{
			ID: "battery", Agent: "triage", By: "platform", Cases: casesOf("estorno"),
		})
		if err != nil || len(opened.Runs) != 1 || opened.Runs[0] == old.RunID {
			t.Fatalf("opened = %+v, err = %v, want a second run", opened, err)
		}

		reached := old.RunID
		if !heldFirst {
			reached = opened.Runs[0]
		}
		appendStep(t, store, reached, domain.StepGateDecided, domain.GateDecidedPayload{
			Tool: "crm.lookup", Effect: domain.EffectRead, Verdict: domain.VerdictAllow,
		})
		appendStep(t, store, reached, domain.StepToolCalled, domain.ToolCalledPayload{})
		for _, run := range []domain.RunID{old.RunID, opened.Runs[0]} {
			appendStep(t, store, run, domain.StepRunFinished, domain.RunFinishedPayload{Outcome: "answered"})
		}

		report, err := simulate.Gather(ctx, store, "battery")
		if err != nil {
			t.Fatalf("Gather: %v", err)
		}
		got := simulate.Battery(report, []domain.RegressionCase{{
			ID: "estorno", Expectations: []domain.Expectation{{Kind: domain.ExpectCalls, Value: "crm.lookup"}},
		}})
		if got.Broken != 1 || got.Held != 0 {
			t.Errorf("heldFirst=%v: broken = %d, held = %d, want it broken", heldFirst, got.Broken, got.Held)
		}
	}
}
