package simulate_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/simulate"
	"github.com/fuseone/agents/internal/trigger"
)

/*
An experiment opens each case once per repetition (NT-015 § Experiments).

Repetitions are for structural certainty: a case broken in any repetition is broken, so
each one has to be its own run, recorded as part of the experiment with its configuration.
Asking again for the same experiment reaches the same runs, as a retried battery does.
*/

func experimentBatch(repetitions int) simulate.Batch {
	return simulate.Batch{
		ID: "exp-1", Agent: "triage", By: "ana", DatasetVersion: 2,
		Cases:      casesOf("a", "b"),
		Experiment: &simulate.Experiment{Config: domain.ExperimentConfig{Effort: "low"}, Repetitions: repetitions},
	}
}

func TestOpen_anExperiment_opensEveryCaseOncePerRepetitionAndRecordsIt(t *testing.T) {
	t.Parallel()
	store := ledger.NewMemory()
	opener := openerFor(store, engine.NewMemoryContent())

	opened, err := simulate.Open(context.Background(), opener, experimentBatch(3))
	if err != nil || len(opened.Failed) != 0 {
		t.Fatalf("Open: %v failed=%v", err, opened.Failed)
	}
	if got := casesRun(t, store, "exp-1"); len(got) != 6 ||
		got[0] != "a" || got[2] != "a" || got[3] != "b" || got[5] != "b" {
		t.Fatalf("cases run = %v, want a and b three times each", got)
	}
	for _, run := range opened.Runs {
		steps, err := store.Read(context.Background(), run, domain.FirstSeq)
		if err != nil {
			t.Fatalf("Read: %v", err)
		}
		started := decodeStarted(t, steps[0])
		if started.Experiment != "exp-1" || started.ExperimentConfig == nil ||
			started.ExperimentConfig.Effort != "low" || started.DatasetVersion != 2 {
			t.Errorf("run %s opened as %+v, want a run of exp-1 under its configuration", run, started)
		}
	}
}

func TestOpen_anExperimentAskedAgain_reachesTheRunsItOpened(t *testing.T) {
	t.Parallel()
	store := ledger.NewMemory()
	opener := openerFor(store, engine.NewMemoryContent())

	for range 2 {
		if _, err := simulate.Open(context.Background(), opener, experimentBatch(2)); err != nil {
			t.Fatalf("Open: %v", err)
		}
	}
	if got := casesRun(t, store, "exp-1"); len(got) != 4 {
		t.Errorf("cases run = %v, want each case twice and no more", got)
	}
}

func TestOpen_anExperiment_outsideOneToFiveRepetitions_opensNothing(t *testing.T) {
	t.Parallel()
	for _, repetitions := range []int{0, 6} {
		store := ledger.NewMemory()
		opener := openerFor(store, engine.NewMemoryContent())

		if _, err := simulate.Open(context.Background(), opener, experimentBatch(repetitions)); err == nil {
			t.Errorf("%d repetitions opened", repetitions)
		}
		if got := casesRun(t, store, "exp-1"); len(got) != 0 {
			t.Errorf("%d repetitions opened %v", repetitions, got)
		}
	}
}

func decodeStarted(t *testing.T, opening domain.Step) domain.RunStartedPayload {
	t.Helper()
	var started domain.RunStartedPayload
	if err := json.Unmarshal(opening.Payload, &started); err != nil {
		t.Fatalf("decode run_started: %v", err)
	}
	return started
}

type twoVersions struct{}

func (twoVersions) Versions(context.Context, domain.AgentID) ([]domain.AgentSummary, error) {
	scope := domain.Scope{Company: "acme", Area: "cx"}
	return []domain.AgentSummary{
		{ID: "triage", VersionID: "v2", Latest: true, Scope: scope},
		{ID: "triage", VersionID: "v1", Scope: scope},
	}, nil
}

// An experiment pins the agent version it names, not the one published now: comparing two
// configurations of one version is the point, and a publish in between would compare two.
func TestOpen_anExperiment_runsTheVersionItNames(t *testing.T) {
	t.Parallel()
	store := ledger.NewMemory()
	opener := trigger.NewOpener(store, twoVersions{}, clock{}).WithContent(engine.NewMemoryContent())
	batch := experimentBatch(1)
	batch.Experiment.Version = "v1"

	opened, err := simulate.Open(context.Background(), opener, batch)
	if err != nil || len(opened.Runs) != 2 {
		t.Fatalf("Open: %v runs=%v failed=%v", err, opened.Runs, opened.Failed)
	}
	for _, run := range opened.Runs {
		steps, err := store.Read(context.Background(), run, domain.FirstSeq)
		if err != nil {
			t.Fatalf("Read: %v", err)
		}
		if steps[0].VersionID != "v1" {
			t.Errorf("run %s opened on %s, want the named v1", run, steps[0].VersionID)
		}
	}
}
