package worker

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/gate"
	"github.com/fuseone/agents/internal/ledger"
)

/*
The simulation pool plans an experiment run on its configuration, and nothing else on
one (NT-015 § Experiments, rule 2).

The opener already refuses a configuration outside an experiment. The worker is the
second door: it honours one only on a simulated run that names an experiment, because a
run that reached the pool some other way with a configuration on it would otherwise plan
on a model nobody published.
*/

// experimentSpecs records which resolution a turn asked for.
type experimentSpecs struct {
	planner  engine.Planner
	asked    []domain.ExperimentConfig
	declared int
}

func (s *experimentSpecs) resolution() Resolution {
	return Resolution{
		Start: engine.Start{
			Pack: gate.NewPack("crm.lookup"), Budget: domain.Budget{Micros: 1_000_000, Steps: 50},
			Trigger: "worker",
		},
		Planner: s.planner,
	}
}

func (s *experimentSpecs) Resolve(context.Context, domain.AgentID, domain.VersionID) (Resolution, error) {
	s.declared++
	return s.resolution(), nil
}

func (s *experimentSpecs) ResolveExperiment(
	_ context.Context, _ domain.AgentID, _ domain.VersionID, config domain.ExperimentConfig,
) (Resolution, error) {
	s.asked = append(s.asked, config)
	return s.resolution(), nil
}

func openSimulated(t *testing.T, store *ledger.Memory, started domain.RunStartedPayload) {
	t.Helper()
	payload, err := json.Marshal(started)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if _, err := store.Append(context.Background(), domain.Step{
		RunID: "run-1", Kind: domain.StepRunStarted, Scope: domain.Scope{Company: "acme", Area: "cx"},
		AgentID: "triage", VersionID: "v3", At: time.Now(), IdemKey: "run-1", Payload: payload,
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
}

func simulationTurn(t *testing.T, store *ledger.Memory, specs *experimentSpecs) {
	t.Helper()
	turnOn(t, store, store.Simulations(), specs)
}

// turnOn runs one turn of a pool with experiments wired, claiming from queue.
func turnOn(t *testing.T, store *ledger.Memory, queue Queue, specs *experimentSpecs) {
	t.Helper()
	clock := frozenClock{t: time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)}
	deps := engine.Deps{
		Ledger: store, Content: engine.NewMemoryContent(), Gate: gate.New(),
		Tools: noTools{}, Catalog: emptyCatalog{}, Clock: clock,
	}
	w := New(Config{Owner: "w-sim"}, queue, deps, specs, clock, slog.New(slog.DiscardHandler)).
		WithStages(stageIs(domain.StageDraft)).WithExperiments(specs)
	claimed, err := w.turn(context.Background(), slog.New(slog.DiscardHandler))
	if err != nil || !claimed {
		t.Fatalf("turn: claimed=%v err=%v", claimed, err)
	}
}

func TestTurn_anExperimentRun_plansOnItsConfiguration(t *testing.T) {
	t.Parallel()
	store := ledger.NewMemory()
	config := domain.ExperimentConfig{Model: "other-model", Effort: "low"}
	openSimulated(t, store, domain.RunStartedPayload{
		Trigger: "simulation", Simulated: true, Simulation: "exp-1", Case: "case-1",
		Experiment: "exp-1", ExperimentConfig: &config,
	})
	specs := &experimentSpecs{planner: &flakyPlanner{}}

	simulationTurn(t, store, specs)

	if len(specs.asked) != 1 || specs.asked[0] != config || specs.declared != 0 {
		t.Errorf("asked %+v and the declared version %d times, want %+v alone",
			specs.asked, specs.declared, config)
	}
}

func TestTurn_aConfigurationOutsideAnExperiment_isNeverHonoured(t *testing.T) {
	t.Parallel()
	forged := &domain.ExperimentConfig{Provider: "openai", Model: "gpt-5.5"}
	cases := []struct {
		name    string
		started domain.RunStartedPayload
	}{
		// What the opener refuses, reaching the pool another way.
		{"a battery carrying a configuration", domain.RunStartedPayload{
			Trigger: "simulation", Simulated: true, Simulation: "sim-1", Case: "case-1",
			ExperimentConfig: forged,
		}},
		// The version as published, fallback and all.
		{"an experiment with no configuration", domain.RunStartedPayload{
			Trigger: "simulation", Simulated: true, Simulation: "exp-1", Case: "case-1",
			Experiment: "exp-1",
		}},
		// The opener records an empty one as none; a record that kept it is still none.
		{"an experiment with an empty configuration", domain.RunStartedPayload{
			Trigger: "simulation", Simulated: true, Simulation: "exp-1", Case: "case-1",
			Experiment: "exp-1", ExperimentConfig: &domain.ExperimentConfig{},
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			store := ledger.NewMemory()
			openSimulated(t, store, c.started)
			specs := &experimentSpecs{planner: &flakyPlanner{}}

			simulationTurn(t, store, specs)

			if len(specs.asked) != 0 || specs.declared != 1 {
				t.Errorf("asked %+v and the declared version %d times, want the declared version alone",
					specs.asked, specs.declared)
			}
		})
	}
}

// A real run that names an experiment and a configuration, claimed by a pool with
// experiments wired, plans as the version published it: the configuration needs a
// simulated run, whatever else the record says.
func TestTurn_aRealRunNamingAnExperiment_plansAsPublished(t *testing.T) {
	t.Parallel()
	store := ledger.NewMemory()
	openSimulated(t, store, domain.RunStartedPayload{
		Trigger: "manual", Experiment: "exp-1",
		ExperimentConfig: &domain.ExperimentConfig{Provider: "openai", Model: "gpt-5.5"},
	})
	specs := &experimentSpecs{planner: &flakyPlanner{}}

	turnOn(t, store, store, specs)

	if len(specs.asked) != 0 || specs.declared != 1 {
		t.Errorf("asked %+v and the declared version %d times, want the declared version alone",
			specs.asked, specs.declared)
	}
}
