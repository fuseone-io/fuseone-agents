package e2e_test

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/gate"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/model"
	"github.com/fuseone/agents/internal/spec"
	"github.com/fuseone/agents/internal/tools"
	"github.com/fuseone/agents/internal/worker"
)

// Store is what the platform asks of a ledger: append and read for the engine,
// claim and release for the worker, verify for the auditor.
type Store interface {
	engine.Ledger
	Verify(ctx context.Context, runID domain.RunID) error
	Claim(ctx context.Context, owner string, lease time.Duration) (domain.Claim, error)
	Release(ctx context.Context, runID domain.RunID, outcome domain.ClaimOutcome) error
}

// eachLedger runs the same scenario against both ledgers. Not redundancy: an
// agent that completes in memory and stalls on Postgres is exactly the
// divergence the in-memory fake exists to rule out.
func eachLedger(t *testing.T, name string, fn func(t *testing.T, store Store)) {
	t.Helper()

	t.Run("memory/"+name, func(t *testing.T) { fn(t, ledger.NewMemory()) })

	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		if os.Getenv("REQUIRE_DATABASE") != "" {
			t.Fatal("REQUIRE_DATABASE is set but TEST_DATABASE_URL is empty")
		}
		t.Log("TEST_DATABASE_URL is unset; skipping the Postgres run")
		return
	}
	t.Run("postgres/"+name, func(t *testing.T) { fn(t, openPostgres(t, dsn)) })
}

func openPostgres(t *testing.T, dsn string) Store {
	t.Helper()
	ctx := context.Background()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	if err := ledger.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if _, err := pool.Exec(ctx, `truncate run_steps, runs`); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	return ledger.NewPostgres(pool)
}

// agentFull grants both tools; agentReadOnly grants only the read. The pack is
// part of the specification, so narrowing it means publishing a different
// version — which is exactly how an operator would do it.
const agentFull = `---
id: triage
name: Triagem
area: cx
provider: openai
model: test-model
effort: low
tools:
  - crm.lookup
  - crm.note
budget:
  micros: 500000
  tool_calls: 20
  steps: 60
---

Find the customer and report the account.
`

const agentReadOnly = `---
id: triage
name: Triagem
area: cx
provider: openai
model: test-model
effort: low
tools:
  - crm.lookup
budget:
  micros: 500000
  tool_calls: 20
  steps: 60
---

Find the customer and report the account.
`

// platform is an installation, assembled.
type platform struct {
	store   Store
	worker  *worker.Worker
	catalog *tools.Catalog
	model   *modelServer
	spec    spec.Spec

	// server is what the MCP server actually executed.
	server serverCalls
}

func newPlatform(t *testing.T, store Store, agentSource string, reply func(turn int) chatReply) *platform {
	t.Helper()
	p := &platform{store: store}

	// A real MCP server, discovered the way a configured one is. Tools arrive
	// classified read-only whatever the server claims; promoting one is the
	// Curator's separate act, so the test has to perform it explicitly.
	// One content store, shared. The catalogue writes tool results into it and
	// the engine resolves them back out when building the next transcript;
	// two instances silently lose every result.
	content := engine.NewMemoryContent()
	p.catalog = tools.NewCatalog(content)
	if err := p.catalog.AddServer(t.Context(), "crm", mcpSession(t, &p.server)); err != nil {
		t.Fatalf("add MCP server: %v", err)
	}

	p.model = newModelServer(t, reply)
	registry := model.NewRegistry(nil)
	if err := registry.Register(model.Provider{
		Name: "openai", Kind: model.KindOpenAICompatible,
		BaseURL: p.model.URL, APIKey: "test-key",
	}); err != nil {
		t.Fatalf("register provider: %v", err)
	}

	specs := spec.NewStore()
	parsed, err := spec.Parse("triage.agent.md", []byte(agentSource))
	if err != nil {
		t.Fatalf("parse spec: %v", err)
	}
	if err := specs.Publish(parsed); err != nil {
		t.Fatalf("publish spec: %v", err)
	}
	p.spec = parsed

	p.worker = worker.New(
		worker.Config{
			Owner: "e2e", Concurrency: 1, Lease: time.Minute,
			IdleWait: time.Millisecond, MaxAttempts: 3,
		},
		store,
		engine.Deps{
			Ledger:  store,
			Gate:    gate.New(),
			Tools:   p.catalog,
			Catalog: p.catalog,
			Clock:   engine.SystemClock{},
			Content: content,
		},
		spec.NewResolver(specs, registry, p.catalog),
		engine.SystemClock{},
		slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn})),
	)
	return p
}

// open records the run the way a trigger would, then lets the worker find it.
func (p *platform) open(t *testing.T, runID domain.RunID) {
	t.Helper()

	if _, err := p.store.Append(t.Context(), domain.Step{
		RunID: runID, Kind: domain.StepRunStarted,
		Scope:     domain.Scope{Company: "acme", Area: "cx"},
		AgentID:   p.spec.ID,
		VersionID: p.spec.Version,
		At:        time.Now(),
	}); err != nil {
		t.Fatalf("open run: %v", err)
	}
}

// settle runs the worker pool until the run stops moving, the way it stops in
// production: on its own, not because the test counted turns.
func (p *platform) settle(t *testing.T, runID domain.RunID) engine.State {
	t.Helper()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	stopped := make(chan struct{})
	go func() { defer close(stopped); _ = p.worker.Run(ctx) }()

	deadline := time.Now().Add(10 * time.Second)
	for {
		if state := p.state(t, runID); settled(state.Phase) {
			cancel()
			<-stopped
			return state
		}
		if time.Now().After(deadline) {
			cancel()
			<-stopped
			t.Fatalf("run never settled; last steps: %v", kindsOf(p.steps(t, runID)))
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// settled means nothing unattended will move this run again.
func settled(p engine.Phase) bool {
	return p == engine.PhaseFinished || p == engine.PhaseParked || p == engine.PhaseAwaitingApproval
}

func (p *platform) steps(t *testing.T, runID domain.RunID) []domain.Step {
	t.Helper()

	steps, err := p.store.Read(t.Context(), runID, domain.FirstSeq)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	return steps
}

func (p *platform) state(t *testing.T, runID domain.RunID) engine.State {
	t.Helper()

	state, err := engine.Fold(p.steps(t, runID))
	if err != nil {
		t.Fatalf("Fold: %v", err)
	}
	return state
}

func kindsOf(steps []domain.Step) []domain.StepKind {
	out := make([]domain.StepKind, 0, len(steps))
	for _, s := range steps {
		out = append(out, s.Kind)
	}
	return out
}
