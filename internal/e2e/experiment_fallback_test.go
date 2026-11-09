package e2e_test

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/gate"
	"github.com/fuseone/agents/internal/httpapi"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/model"
	"github.com/fuseone/agents/internal/simulate"
	"github.com/fuseone/agents/internal/spec"
	"github.com/fuseone/agents/internal/testdb"
	"github.com/fuseone/agents/internal/tools"
	"github.com/fuseone/agents/internal/worker"
)

/*
An experiment and the declared fallback, with the declared provider down (NT-015, and
the decision recorded on #297).

An empty configuration is the version as published, so its declared fallback takes over
the way it would in production. Any other configuration, effort alone included, keeps
asking the model it chose until the retries run out and then parks: a fallback that took
over would answer, and the result would say the configuration that was tested passed.

Both cases cross the publish route, the registry, the resolver and the simulation pool,
with nothing injected between them.
*/
func TestExperiment_withTheDeclaredProviderDown_onlyAnEmptyConfigurationFallsBack(t *testing.T) {
	cases := []struct {
		name         string
		config       *domain.ExperimentConfig
		want         engine.Phase
		fallbackUsed bool
	}{
		{"an empty configuration", nil, engine.PhaseFinished, true},
		{"only effort", &domain.ExperimentConfig{Effort: "low"}, engine.PhaseParked, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p, backup := experimentPlatform(t)
			openExperimentRun(t, p, "run-experiment", c.config)

			state := p.settle(t, "run-experiment")
			steps := p.steps(t, "run-experiment")

			if state.Phase != c.want {
				t.Fatalf("Phase = %v, want %v; steps %v", state.Phase, c.want, kindsOf(steps))
			}
			if used := len(backup.requests()) > 0; used != c.fallbackUsed {
				t.Errorf("the fallback was asked: %v, want %v", used, c.fallbackUsed)
			}
			if switched := countKind(steps, domain.StepModelFallback) > 0; switched != c.fallbackUsed {
				t.Errorf("the ledger records a switch: %v, want %v", switched, c.fallbackUsed)
			}
		})
	}
}

// experimentPlatform publishes a version that declares a fallback, with its own provider
// down, and builds the simulation pool the way the worker command does.
func experimentPlatform(t *testing.T) (*platform, *modelServer) {
	t.Helper()
	pool := testdb.Pool(t)
	primary := newModelServer(t, func(int) chatReply { return chatReply{Text: "primary answered"} })
	primary.fail()
	backup := newModelServer(t, func(int) chatReply { return chatReply{Text: "backup answered"} })
	providers := model.NewRegistry(nil)
	for _, p := range []model.Provider{
		{Name: "primary", Kind: model.KindOpenAICompatible, BaseURL: primary.URL, APIKey: "k"},
		{Name: "backup", Kind: model.KindOpenAICompatible, BaseURL: backup.URL, APIKey: "k"},
	} {
		if err := providers.Register(p); err != nil {
			t.Fatalf("register %s: %v", p.Name, err)
		}
	}
	agent := fmt.Sprintf("experiment-fallback-%d", time.Now().UnixNano())
	server := httpapi.NewServer(ledger.NewMemory(), "test").
		WithPublisher(spec.NewPublisher(pool, engine.SystemClock{})).WithProviderNames(providers)
	version := publishThroughTheRoute(t, server, agent, `{
		"name": "Journey", "company": "acme", "area": "cx",
		"provider": "primary", "model": "primary-model",
		"fallback": {"provider": "backup", "model": "backup-model"},
		"instructions": "Answer the ticket.", "tools": ["crm.lookup"],
		"budget": {"micros": 500000, "steps": 60}
	}`)

	store, content := ledger.NewMemory(), engine.NewMemoryContent()
	var calls serverCalls
	catalog := tools.NewCatalog(content)
	if err := catalog.AddServer(t.Context(), "crm", mcpSession(t, &calls), nil); err != nil {
		t.Fatalf("add MCP server: %v", err)
	}
	resolver := spec.NewResolver(spec.NewRegistry(pool), providers, catalog)
	deps := engine.Deps{Ledger: store, Gate: gate.New(), Tools: catalog, Catalog: catalog,
		Clock: engine.SystemClock{}, Content: content}
	w := worker.New(
		worker.Config{Owner: "e2e-sim", Concurrency: 1, Lease: time.Minute, IdleWait: time.Millisecond,
			MaxAttempts: 3, BaseBackoff: time.Millisecond, MaxBackoff: 5 * time.Millisecond},
		store.Simulations(), simulate.Deps(deps), resolver, engine.SystemClock{},
		slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
	).WithStages(copilot{}).WithExperiments(resolver)
	return &platform{store: store, worker: w, spec: spec.Spec{ID: domain.AgentID(agent), Version: version}}, backup
}

func openExperimentRun(t *testing.T, p *platform, run domain.RunID, config *domain.ExperimentConfig) {
	t.Helper()
	payload, err := json.Marshal(domain.RunStartedPayload{
		Trigger: "simulation", Simulated: true, Simulation: "exp-1", Case: "case-1",
		Experiment: "exp-1", ExperimentConfig: config,
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if _, err := p.store.Append(t.Context(), domain.Step{
		RunID: run, Kind: domain.StepRunStarted, Scope: domain.Scope{Company: "acme", Area: "cx"},
		AgentID: p.spec.ID, VersionID: p.spec.Version, OnBehalfOf: "ana", At: time.Now(),
		IdemKey: string(run), Payload: payload,
	}); err != nil {
		t.Fatalf("open run: %v", err)
	}
}

func countKind(steps []domain.Step, kind domain.StepKind) int {
	n := 0
	for _, s := range steps {
		if s.Kind == kind {
			n++
		}
	}
	return n
}
