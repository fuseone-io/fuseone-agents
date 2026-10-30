package e2e_test

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/gate"
	"github.com/fuseone/agents/internal/httpapi"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/model"
	"github.com/fuseone/agents/internal/spec"
	"github.com/fuseone/agents/internal/tools"
	"github.com/fuseone/agents/internal/worker"
)

/*
The fallback pair crosses every layer a definition crosses before a run uses
it: the console's JSON, the publish route's render-and-parse, the registry's
columns, the resolver's planner cache, the worker's supervision and the engine's
fold. Each is a hand-written conversion, and each is a place a new field
silently dies — so this starts at the publish route and ends at the provider
the fallback names, injecting nothing in between.
*/

type copilot struct{}

func (copilot) StageOf(context.Context, domain.AgentID) (domain.Stage, error) {
	return domain.StageCopilot, nil
}

func openSpecs(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		if os.Getenv("REQUIRE_DATABASE") != "" {
			t.Fatal("REQUIRE_DATABASE is set but TEST_DATABASE_URL is empty")
		}
		t.Skip("TEST_DATABASE_URL is unset; skipping the publish journey")
	}
	pool, err := pgxpool.New(t.Context(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := ledger.Migrate(t.Context(), pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return pool
}

// publishThroughTheRoute sends the definition the way the console does: JSON
// decoded into the wire type, handed to the publish route by an author.
func publishThroughTheRoute(t *testing.T, server *httpapi.Server, agent, body string) domain.VersionID {
	t.Helper()
	var definition openapi.AgentDefinition
	if err := json.Unmarshal([]byte(body), &definition); err != nil {
		t.Fatalf("decode the console's body: %v", err)
	}
	author := auth.WithPrincipal(t.Context(), domain.Principal{
		ID: "usr_ana", Kind: domain.PrincipalUser,
		Grants: []domain.Grant{{Scope: domain.Scope{Company: "acme", Area: "cx"}, Role: domain.RoleAuthor}},
	})
	resp, err := server.PublishAgent(author, openapi.PublishAgentRequestObject{AgentId: agent, Body: &definition})
	if err != nil {
		t.Fatalf("PublishAgent: %v", err)
	}
	ok, published := resp.(openapi.PublishAgent200JSONResponse)
	if !published {
		t.Fatalf("publish answered %T: %+v", resp, resp)
	}
	return domain.VersionID(ok.VersionId)
}

func TestFallback_publishedFromTheConsole_reachesTheProviderItNames(t *testing.T) {
	pool := openSpecs(t)

	// Two providers, one of them down the way a real one is down.
	primary := newModelServer(t, func(int) chatReply { return chatReply{Text: "primary answered"} })
	primary.fail()
	backup := newModelServer(t, func(int) chatReply {
		return chatReply{Text: "backup answered", PromptTokens: 1_000, CompletionTokens: 10}
	})
	providers := model.NewRegistry(nil)
	for _, p := range []model.Provider{
		{Name: "primary", Kind: model.KindOpenAICompatible, BaseURL: primary.URL, APIKey: "k"},
		{Name: "backup", Kind: model.KindOpenAICompatible, BaseURL: backup.URL, APIKey: "k",
			Prices: map[string]model.Prices{"backup-model": {InputMicros: 3_000_000}}},
	} {
		if err := providers.Register(p); err != nil {
			t.Fatalf("register %s: %v", p.Name, err)
		}
	}

	// A fresh agent per run of this test: the registry is shared, and a
	// version is the digest of its text.
	agent := fmt.Sprintf("fallback-journey-%d", time.Now().UnixNano())
	server := httpapi.NewServer(ledger.NewMemory(), "test").
		WithPublisher(spec.NewPublisher(pool, engine.SystemClock{})).
		WithProviderNames(providers)
	version := publishThroughTheRoute(t, server, agent, `{
		"name": "Journey", "company": "acme", "area": "cx",
		"provider": "primary", "model": "primary-model",
		"fallback": {"provider": "backup", "model": "backup-model"},
		"instructions": "Answer the ticket.",
		"tools": ["crm.lookup"],
		"budget": {"micros": 500000, "steps": 60}
	}`)

	store := ledger.NewMemory()
	content := engine.NewMemoryContent()
	var calls serverCalls
	catalog := tools.NewCatalog(content)
	if err := catalog.AddServer(t.Context(), "crm", mcpSession(t, &calls), nil); err != nil {
		t.Fatalf("add MCP server: %v", err)
	}
	w := worker.New(
		worker.Config{Owner: "e2e", Concurrency: 1, Lease: time.Minute, IdleWait: time.Millisecond,
			MaxAttempts: 3, BaseBackoff: time.Millisecond, MaxBackoff: 5 * time.Millisecond},
		store,
		engine.Deps{Ledger: store, Gate: gate.New(), Tools: catalog, Catalog: catalog,
			Clock: engine.SystemClock{}, Content: content},
		spec.NewResolver(spec.NewRegistry(pool), providers, catalog),
		engine.SystemClock{},
		slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
	).WithStages(copilot{})

	p := &platform{store: store, worker: w, spec: spec.Spec{ID: domain.AgentID(agent), Version: version}}
	p.open(t, "run-fallback")
	if state := p.settle(t, "run-fallback"); state.Phase != engine.PhaseFinished {
		t.Fatalf("Phase = %v, want the fallback to have finished the run; steps %v",
			state.Phase, kindsOf(p.steps(t, "run-fallback")))
	}

	asked := backup.requests()
	if len(asked) != 1 || asked[0].Model != "backup-model" {
		t.Fatalf("backup was asked %+v, want one request for backup-model", asked)
	}
	assertSwitchedAndBilledUnderTheFallback(t, p.steps(t, "run-fallback"))
}

func assertSwitchedAndBilledUnderTheFallback(t *testing.T, steps []domain.Step) {
	t.Helper()
	var switched, planned int
	for _, s := range steps {
		switch s.Kind {
		case domain.StepModelFallback:
			switched++
			var p domain.ModelFallbackPayload
			if err := json.Unmarshal(s.Payload, &p); err != nil {
				t.Fatalf("payload: %v", err)
			}
			if p.From.Provider != "primary" || p.To.Provider != "backup" ||
				p.Failure.Code != model.CodeProviderUnavailable {
				t.Errorf("switch = %+v", p)
			}
		case domain.StepPlanned:
			planned++
			var p domain.PlannedPayload
			if err := json.Unmarshal(s.Payload, &p); err != nil {
				t.Fatalf("payload: %v", err)
			}
			// FinOps groups planned steps by the pair they name, so this is
			// where the fallback's spend lands — unchanged code, new pair.
			if p.Provider != "backup" || p.Model != "backup-model" || s.Cost.Micros != 3_000 {
				t.Errorf("planned = %s/%s at %d micros, want backup/backup-model at its own rate",
					p.Provider, p.Model, s.Cost.Micros)
			}
		}
	}
	if switched != 1 || planned != 1 {
		t.Errorf("switched %d / planned %d times, want 1 / 1", switched, planned)
	}
}
