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

// Only an experiment's runs are an experiment's record. A battery's id names runs too, and
// reading them as an experiment would report a battery as one, under no configuration.
func TestRecorded_runsThatAreNotThatExperiments_areNotFound(t *testing.T) {
	t.Parallel()
	store := ledger.NewMemory()
	opener := openerFor(store, engine.NewMemoryContent())
	if _, err := simulate.Open(t.Context(), opener, batchOf(`{"n":1}`)); err != nil {
		t.Fatalf("Open: %v", err)
	}

	if _, found, err := simulate.Recorded(t.Context(), store, "sim-1"); err != nil || found {
		t.Errorf("Recorded(a battery) = %v found=%v, want it not found", err, found)
	}
}

// The record names the agent its runs belong to, so a route under one agent never reads
// another's experiment.
func TestRecorded_namesTheAgentItsRunsBelongTo(t *testing.T) {
	t.Parallel()
	store := ledger.NewMemory()
	if _, err := simulate.Open(t.Context(), openerFor(store, engine.NewMemoryContent()), experimentBatch(1)); err != nil {
		t.Fatalf("Open: %v", err)
	}

	got, _, err := simulate.Recorded(t.Context(), store, "exp-1")
	if err != nil || got.Agent != "suporte" {
		t.Errorf("Recorded = %q err=%v, want the agent the opener resolved", got.Agent, err)
	}
}

// The record keeps the scope its runs were opened under. An agent republished elsewhere is
// read under its new scope, and its old experiment is not: whoever may read the new one has
// no claim on runs recorded under the old.
func TestRecorded_keepsTheScopeItsRunsWereOpenedUnder(t *testing.T) {
	t.Parallel()
	store := ledger.NewMemory()
	if _, err := simulate.Open(t.Context(), openerFor(store, engine.NewMemoryContent()), experimentBatch(1)); err != nil {
		t.Fatalf("Open: %v", err)
	}

	got, _, err := simulate.Recorded(t.Context(), store, "exp-1")
	if err != nil || got.Scope != (domain.Scope{Company: "acme", Area: "cx"}) {
		t.Errorf("Recorded scope = %+v err=%v, want the scope the opener resolved", got.Scope, err)
	}
}
