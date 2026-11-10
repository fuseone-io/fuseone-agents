package main

import (
	"context"
	"encoding/json"

	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/gate"
	"github.com/fuseone/agents/internal/httpapi"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/model"
	"github.com/fuseone/agents/internal/regression"
	"github.com/fuseone/agents/internal/spec"
	"github.com/fuseone/agents/internal/testdb"
	"github.com/fuseone/agents/internal/tools"
	"github.com/fuseone/agents/internal/worker"
)

/*
An experiment, from the route to the provider it names (NT-015 § Experiments).

Nothing is injected in between. The configuration crosses the console's JSON, the route,
the opener's fence, the ledger, the simulation pool the worker command builds, the
resolver and the planner, and every one of those is a conversion where a field can die
quietly: an experiment that ran on the version's own model would still finish, and its
result would say it tested another.
*/

// finishingModel answers every call by finishing, and remembers which model was asked.
type finishingModel struct {
	mu    sync.Mutex
	asked []string
}

func (m *finishingModel) serve(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Model string `json:"model"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		m.mu.Lock()
		m.asked = append(m.asked, body.Model)
		m.mu.Unlock()
		args, _ := json.Marshal(map[string]string{"summary": "classified"})
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"finish_reason": "tool_calls", "message": map[string]any{
				"role": "assistant", "tool_calls": []map[string]any{{
					"id": "call_finish", "type": "function",
					"function": map[string]any{"name": "_fuseone__finish", "arguments": string(args)},
				}},
			}}},
			"usage": map[string]any{"prompt_tokens": 10, "completion_tokens": 2},
		})
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func (m *finishingModel) models() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.asked...)
}

type experimentWorld struct {
	pool             *pgxpool.Pool
	store            *ledger.Postgres
	server           *httpapi.Server
	resolver         *spec.Resolver
	catalog          *tools.Catalog
	declared, tested *finishingModel
	agent            domain.AgentID
}

func newExperimentWorld(t *testing.T) *experimentWorld {
	t.Helper()
	pool := testdb.Isolated(t)
	w := &experimentWorld{
		pool: pool, store: ledger.NewPostgres(pool),
		declared: &finishingModel{}, tested: &finishingModel{}, agent: "triage",
	}
	providers := model.NewRegistry(nil)
	for name, m := range map[string]*finishingModel{"primary": w.declared, "elsewhere": w.tested} {
		if err := providers.Register(model.Provider{
			Name: name, Kind: model.KindOpenAICompatible, BaseURL: m.serve(t), APIKey: "k",
		}); err != nil {
			t.Fatalf("register %s: %v", name, err)
		}
	}
	content := ledger.NewContent(pool)
	w.server = httpapi.NewServer(w.store, "test").
		WithPublisher(spec.NewPublisher(pool, engine.SystemClock{})).
		WithAgents(spec.NewRegistry(pool)).
		WithContent(content).WithCases(content).
		WithRegressions(regression.NewStore(pool)).
		WithProviderNames(providers)
	w.publish(t)
	w.correct(t, content)
	// The one tool the version declares, so the planner can name it to the model.
	w.catalog = tools.NewCatalog(content)
	if err := w.catalog.AddServer(t.Context(), "crm",
		&testSession{tools: []*mcp.Tool{{Name: "lookup"}}}, nil); err != nil {
		t.Fatalf("AddServer: %v", err)
	}
	w.resolver = spec.NewResolver(spec.NewRegistry(pool), providers, w.catalog)
	return w
}

func author(ctx context.Context) context.Context {
	return auth.WithPrincipal(ctx, domain.Principal{
		ID: "usr_ana", Kind: domain.PrincipalUser,
		Grants: []domain.Grant{{Scope: domain.Scope{Company: "acme", Area: "cx"}, Role: domain.RoleAuthor}},
	})
}

func (w *experimentWorld) publish(t *testing.T) {
	t.Helper()
	var definition openapi.AgentDefinition
	if err := json.Unmarshal([]byte(`{
		"name": "Triage", "company": "acme", "area": "cx",
		"provider": "primary", "model": "declared-model",
		"instructions": "Classify the ticket.", "tools": ["crm.lookup"],
		"budget": {"micros": 500000, "steps": 20}
	}`), &definition); err != nil {
		t.Fatalf("decode: %v", err)
	}
	resp, err := w.server.PublishAgent(author(t.Context()),
		openapi.PublishAgentRequestObject{AgentId: string(w.agent), Body: &definition})
	if err != nil {
		t.Fatalf("PublishAgent: %v", err)
	}
	if _, ok := resp.(openapi.PublishAgent200JSONResponse); !ok {
		t.Fatalf("publish answered %T: %+v", resp, resp)
	}
}

// correct records the one case the experiment replays, the way a correction is kept.
func (w *experimentWorld) correct(t *testing.T, content *ledger.Content) {
	t.Helper()
	ref, err := content.RestoreFor(t.Context(), "regression", string(w.agent), 1, []byte(`{"ask":"refund"}`))
	if err != nil {
		t.Fatalf("keep the occurrence: %v", err)
	}
	if err := regression.NewStore(w.pool).Record(t.Context(), domain.RegressionCase{
		ID: "refund", Agent: w.agent, Scope: domain.Scope{Company: "acme", Area: "cx"},
		InputRef: ref, FromRun: "run-origin",
		Expectations: []domain.Expectation{{Kind: domain.ExpectNeverCalls, Value: "crm.refund"}},
	}); err != nil {
		t.Fatalf("Record: %v", err)
	}
}

// settle runs the simulation pool the worker command builds until every run settles.
func (w *experimentWorld) settle(t *testing.T, runs []domain.RunID) {
	t.Helper()
	parts := &workerParts{store: w.store, queue: w.store, catalog: w.catalog}
	deps := engine.Deps{Ledger: w.store, Gate: gate.New(), Tools: w.catalog, Catalog: w.catalog,
		Clock: engine.SystemClock{}, Content: ledger.NewContent(w.pool)}
	_, sim := parts.pools(workerFlags{owner: "journey", concurrency: 1, lease: time.Minute},
		deps, w.resolver, worker.NewMetricsRegistry())

	ctx, cancel := context.WithCancel(t.Context())
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		if err := sim.Run(ctx); err != nil && ctx.Err() == nil {
			t.Errorf("the simulation pool stopped: %v", err)
		}
	}()
	defer func() { cancel(); <-stopped }()
	for deadline := time.Now().Add(20 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		if w.allFinished(t, runs) {
			return
		}
	}
	for _, run := range runs {
		steps, _ := w.store.Read(t.Context(), run, domain.FirstSeq)
		for _, st := range steps {
			t.Logf("%s %d %s %s", run, st.Seq, st.Kind, st.Payload)
		}
	}
	t.Fatalf("the experiment's runs never finished")
}

func (w *experimentWorld) allFinished(t *testing.T, runs []domain.RunID) bool {
	t.Helper()
	for _, run := range runs {
		steps, err := w.store.Read(t.Context(), run, domain.FirstSeq)
		if err != nil {
			t.Fatalf("Read: %v", err)
		}
		state, err := engine.Fold(steps)
		if err != nil {
			t.Fatalf("Fold: %v", err)
		}
		if state.Phase != engine.PhaseFinished {
			return false
		}
	}
	return true
}

func TestExperiment_fromTheRoute_plansOnTheModelItNamesAndIsNeverABattery(t *testing.T) {
	world := newExperimentWorld(t)
	config := openapi.ExperimentConfig{Provider: ptrTo("elsewhere"), Model: ptrTo("tested-model")}

	resp, err := world.server.StartExperiment(author(t.Context()), openapi.StartExperimentRequestObject{
		AgentId: string(world.agent),
		Params:  openapi.StartExperimentParams{IdempotencyKey: "journey-experiment-1"},
		Body:    &openapi.ExperimentRequest{Config: &config, Repetitions: ptrTo(2)},
	})
	if err != nil {
		t.Fatalf("StartExperiment: %v", err)
	}
	accepted, ok := resp.(openapi.StartExperiment202JSONResponse)
	if !ok || accepted.Runs != 2 {
		t.Fatalf("StartExperiment answered %T %+v, want two runs", resp, resp)
	}
	runs, err := world.store.SimulationRuns(t.Context(), accepted.Id)
	if err != nil || len(runs) != 2 {
		t.Fatalf("runs = %v err=%v", runs, err)
	}
	world.settle(t, runs)

	if got := world.tested.models(); len(got) != 2 || got[0] != "tested-model" || got[1] != "tested-model" {
		t.Errorf("the tested provider was asked %v, want tested-model once per repetition", got)
	}
	if got := world.declared.models(); len(got) != 0 {
		t.Errorf("the version's own provider was asked %v, want nothing", got)
	}
	assertReported(t, world, accepted.Id)
	assertNotABattery(t, world)
}

// The report reads the runs back from Postgres with the configuration visible, and the one
// case ran twice, on the model the experiment named (NT-015, "Journey").
func assertReported(t *testing.T, world *experimentWorld, id string) {
	t.Helper()
	resp, err := world.server.GetExperiment(author(t.Context()),
		openapi.GetExperimentRequestObject{AgentId: string(world.agent), ExperimentId: id})
	if err != nil {
		t.Fatalf("GetExperiment: %v", err)
	}
	report, ok := resp.(openapi.GetExperiment200JSONResponse)
	if !ok {
		t.Fatalf("GetExperiment answered %T %+v", resp, resp)
	}
	got := report.Summary
	if got.Config.Provider == nil || *got.Config.Provider != "elsewhere" || got.Config.Model == nil ||
		*got.Config.Model != "tested-model" || got.Repetitions != 2 || got.CasesRun != 1 ||
		got.CasesInVersion != 1 || got.Running {
		t.Errorf("summary = %+v, want the configuration, two repetitions of the one case, settled", got)
	}
	if len(report.Cases) != 1 || report.Cases[0].Ran != 2 || report.Cases[0].Case.Model == nil ||
		*report.Cases[0].Case.Model != "tested-model" {
		t.Errorf("cases = %+v, want the one case run twice on tested-model", report.Cases)
	}
}

// What the experiment must not be, read through the same doors the start gate, the drift
// sweep and FU-10 read.
func assertNotABattery(t *testing.T, world *experimentWorld) {
	t.Helper()
	versions, err := spec.NewRegistry(world.pool).Versions(t.Context(), world.agent)
	if err != nil || len(versions) == 0 {
		t.Fatalf("Versions: %v err=%v", versions, err)
	}
	if _, found, err := world.store.Latest(t.Context(), world.agent, versions[0].VersionID); err != nil || found {
		t.Errorf("Latest found=%v err=%v, want no battery", found, err)
	}
	if simulated, err := world.store.HasSimulation(t.Context(), world.agent); err != nil || simulated {
		t.Errorf("HasSimulation = %v err=%v, want false", simulated, err)
	}
}
