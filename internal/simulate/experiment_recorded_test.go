package simulate_test

import (
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/simulate"
)

/*
What an experiment's first attempt asked for is read back from the runs it opened.

A retry has to run the experiment it names, not the one it would describe today: the
dataset may have a newer version, the agent may have a newer one, and a caller may have
changed the body. Every run records the whole intention, so any one of them answers,
including the version the opener pinned when the request named none.
*/
func TestRecorded_anExperiment_isWhatItsRunsWereOpenedUnder(t *testing.T) {
	t.Parallel()
	store := ledger.NewMemory()
	batch := experimentBatch(3)
	batch.Experiment.Config = domain.ExperimentConfig{Model: "other-model", Effort: "low"}
	if _, err := simulate.Open(t.Context(), openerFor(store, engine.NewMemoryContent()), batch); err != nil {
		t.Fatalf("Open: %v", err)
	}

	got, found, err := simulate.Recorded(t.Context(), store, "exp-1")
	if err != nil || !found {
		t.Fatalf("Recorded = %v found=%v, want the first attempt", err, found)
	}
	want := simulate.Intention{
		Experiment: simulate.Experiment{
			Version: "v1", Config: domain.ExperimentConfig{Model: "other-model", Effort: "low"}, Repetitions: 3,
		},
		DatasetVersion: 2,
	}
	if got.Intention != want {
		t.Errorf("Recorded = %+v, want %+v", got.Intention, want)
	}
}

// A retry the installation can no longer add to still has an answer: the runs the
// experiment opened, and how many of its cases they cover.
func TestRecorded_anExperiment_countsTheRunsAndCasesItOpened(t *testing.T) {
	t.Parallel()
	store := ledger.NewMemory()
	opened, err := simulate.Open(t.Context(), openerFor(store, engine.NewMemoryContent()), experimentBatch(3))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	got, _, err := simulate.Recorded(t.Context(), store, "exp-1")
	if err != nil {
		t.Fatalf("Recorded: %v", err)
	}
	if len(got.Runs) != len(opened.Runs) || len(got.Runs) != 6 || got.Cases != 2 {
		t.Errorf("Recorded = %d runs over %d cases, want 6 runs over 2", len(got.Runs), got.Cases)
	}
}

func TestRecorded_anExperimentNeverOpened_isNotFound(t *testing.T) {
	t.Parallel()
	_, found, err := simulate.Recorded(t.Context(), ledger.NewMemory(), "exp-1")
	if err != nil || found {
		t.Errorf("Recorded = %v found=%v, want nothing recorded", err, found)
	}
}
