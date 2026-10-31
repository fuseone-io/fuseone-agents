package e2e_test

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/gate"
	"github.com/fuseone/agents/internal/httpapi"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/model"
	"github.com/fuseone/agents/internal/spec"
	"github.com/fuseone/agents/internal/standing"
	"github.com/fuseone/agents/internal/testdb"
	"github.com/fuseone/agents/internal/tools"
	"github.com/fuseone/agents/internal/worker"
)

/*
standing_approvals: forbidden crosses the same layers the fallback pair does —
the console's JSON, the publish route's render-and-parse, the registry's column,
the resolver's Start — before the engine reads it at the one place a grant
could release a write. Each is a hand-written conversion and each is a place
the setting could die quietly, leaving every grant still releasing the run. So
this starts at the publish route and ends at the grant's daily uses, with the
real standing store in between and nothing injected.

The grant is made before the agent forbids standing approvals, which is the
case the creation-side check cannot catch: only the claim side stands between
it and the run.
*/

// standingPort is the engine's port over the real store, as cmd/agentd wires it.
type standingPort struct{ store *standing.Store }

func (p standingPort) Claim(
	ctx context.Context, tool domain.ToolID, agent domain.AgentID,
	scope domain.Scope, run domain.RunID, at time.Time,
) (engine.StandingGrant, bool, error) {
	g, ok, err := p.store.Claim(ctx, tool, agent, scope, run, at)
	if err != nil || !ok {
		return engine.StandingGrant{}, false, err
	}
	return engine.StandingGrant{ID: g.ID, By: g.CreatedBy, Reason: g.Reason}, true, nil
}

func TestStanding_forbiddenFromTheConsole_aGrantMadeBeforeIsNeverClaimed(t *testing.T) {
	j := newStandingJourney(t, `"standingApprovals": "forbidden",`)

	if state := j.runUntil(t, engine.PhaseAwaitingApproval, 300*time.Millisecond); state.Phase != engine.PhaseAwaitingApproval {
		t.Fatalf("Phase = %v, want the write waiting for a person; steps %v",
			state.Phase, kindsOf(j.platform.steps(t, j.runID)))
	}
	if uses := j.usesToday(t); uses != 0 {
		t.Fatalf("grant uses today = %d, want none spent on a run it may not release", uses)
	}
	if notes := j.platform.server.Notes(); len(notes) != 0 {
		t.Fatalf("the write reached the server without a person: %v", notes)
	}
}

// The anchor: the same journey without the line is released by the same
// grant, so the refusal above is the setting's doing and not a set-up that
// could never have been released.
func TestStanding_noSettingFromTheConsole_theGrantReleasesTheWrite(t *testing.T) {
	j := newStandingJourney(t, "")

	if state := j.runUntil(t, engine.PhaseFinished, 0); state.Phase != engine.PhaseFinished {
		t.Fatalf("Phase = %v, want the grant to have released the write; steps %v",
			state.Phase, kindsOf(j.platform.steps(t, j.runID)))
	}
	if uses := j.usesToday(t); uses != 1 {
		t.Fatalf("grant uses today = %d, want the one use the release spent", uses)
	}
}

type standingJourney struct {
	pool     *pgxpool.Pool
	platform *platform
	grantID  string
	runID    domain.RunID
}

func newStandingJourney(t *testing.T, setting string) *standingJourney {
	t.Helper()
	pool := testdb.Pool(t)
	agent := fmt.Sprintf("standing-journey-%d", time.Now().UnixNano())
	j := &standingJourney{pool: pool, grantID: "SG-" + agent, runID: domain.RunID("run-" + agent)}

	// Granted while nothing forbade it: the agent is not yet published.
	store := standing.NewStore(pool)
	now := time.Now()
	if err := store.Create(t.Context(), standing.Grant{
		ID: j.grantID, Tool: "crm.note", Agent: domain.AgentID(agent),
		Scope: domain.Scope{Company: "acme", Area: "cx"}, DailyCap: 5,
		Reason: "night shift notes", CreatedBy: "usr_ana",
		CreatedAt: now, ExpiresAt: now.Add(24 * time.Hour),
	}); err != nil {
		t.Fatalf("Create the grant: %v", err)
	}

	model := newModelServer(t, readThenWrite)
	providers := registerProvider(t, model.URL)
	server := httpapi.NewServer(ledger.NewMemory(), "test").
		WithPublisher(spec.NewPublisher(pool, engine.SystemClock{})).
		WithProviderNames(providers)
	version := publishThroughTheRoute(t, server, agent, `{
		"name": "Journey", "company": "acme", "area": "cx",
		"provider": "primary", "model": "primary-model", `+setting+`
		"instructions": "Note what the customer said.",
		"tools": ["crm.lookup", "crm.note"],
		"budget": {"micros": 500000, "steps": 60}
	}`)

	j.platform = standingPlatform(t, store, providers, pool)
	j.platform.spec = spec.Spec{ID: domain.AgentID(agent), Version: version}
	return j
}

func registerProvider(t *testing.T, url string) *model.Registry {
	t.Helper()
	providers := model.NewRegistry(nil)
	if err := providers.Register(model.Provider{
		Name: "primary", Kind: model.KindOpenAICompatible, BaseURL: url, APIKey: "k",
	}); err != nil {
		t.Fatalf("register: %v", err)
	}
	return providers
}

func standingPlatform(t *testing.T, store *standing.Store, providers *model.Registry, pool *pgxpool.Pool) *platform {
	t.Helper()
	p := &platform{store: ledger.NewMemory(), content: engine.NewMemoryContent()}
	p.catalog = tools.NewCatalog(p.content)
	if err := p.catalog.AddServer(t.Context(), "crm", mcpSession(t, &p.server), nil); err != nil {
		t.Fatalf("add MCP server: %v", err)
	}
	// Reading from outside taints the run, so the note asks for a person.
	for _, c := range []domain.ToolClassification{
		{Tool: "crm.lookup", Effect: domain.EffectRead, Untrusted: true},
		{Tool: "crm.note", Effect: domain.EffectWrite},
	} {
		if err := p.catalog.Classify(c); err != nil {
			t.Fatalf("classify %s: %v", c.Tool, err)
		}
	}
	p.worker = worker.New(
		worker.Config{Owner: "e2e", Concurrency: 1, Lease: time.Minute, IdleWait: time.Millisecond,
			MaxAttempts: 3, BaseBackoff: time.Millisecond, MaxBackoff: 5 * time.Millisecond},
		p.store,
		engine.Deps{Ledger: p.store, Gate: gate.New(), Tools: p.catalog, Catalog: p.catalog,
			Clock: engine.SystemClock{}, Content: p.content, Standing: standingPort{store: store}},
		spec.NewResolver(spec.NewRegistry(pool), providers, p.catalog),
		engine.SystemClock{},
		slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
	).WithStages(copilot{})
	return p
}

/*
runUntil drives the worker until the run reaches want, keeps it running for
grace, and returns the run as it then stands.

Not settle: settle stops the worker the moment the run awaits approval, which
is between the park and the claim of the same turn. Cancelled there, the
claim fails on a dead context — the forbidden journey would pass with the
engine's check removed, and the anchor would never be released. The grace is
what gives a claim that should not happen the time to happen.
*/
func (j *standingJourney) runUntil(t *testing.T, want engine.Phase, grace time.Duration) engine.State {
	t.Helper()
	j.platform.open(t, j.runID)

	ctx, cancel := context.WithCancel(t.Context())
	stopped := make(chan struct{})
	go func() { defer close(stopped); _ = j.platform.worker.Run(ctx) }()
	defer func() { cancel(); <-stopped }()

	deadline := time.Now().Add(10 * time.Second)
	for j.platform.state(t, j.runID).Phase != want {
		if time.Now().After(deadline) {
			t.Fatalf("run never reached %v; steps %v", want, kindsOf(j.platform.steps(t, j.runID)))
		}
		time.Sleep(2 * time.Millisecond)
	}
	time.Sleep(grace)
	return j.platform.state(t, j.runID)
}

func (j *standingJourney) usesToday(t *testing.T) int {
	t.Helper()
	var n int
	if err := j.pool.QueryRow(t.Context(),
		`select count(*) from admin_events where action = $1 and target = $2`,
		standing.UsedAction, j.grantID).Scan(&n); err != nil {
		t.Fatalf("count uses: %v", err)
	}
	return n
}
