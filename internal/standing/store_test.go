package standing_test

import (
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/standing"
)

func storeFor(t *testing.T) (*standing.Store, *pgxpool.Pool) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		if os.Getenv("REQUIRE_DATABASE") != "" {
			t.Fatal("REQUIRE_DATABASE is set but TEST_DATABASE_URL is empty")
		}
		t.Skip("TEST_DATABASE_URL is unset; skipping the standing suite")
	}
	pool, err := pgxpool.New(t.Context(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := ledger.Migrate(t.Context(), pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if _, err := pool.Exec(t.Context(),
		`truncate standing_approvals; delete from admin_events where action like 'standing_approval%'`); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	return standing.NewStore(pool), pool
}

var day0 = time.Date(2026, 2, 1, 8, 0, 0, 0, time.UTC)

func grant(over func(*standing.Grant)) standing.Grant {
	g := standing.Grant{
		ID: "SG-1", Tool: "cloudflare.edge.block_ip", Agent: "sentinel",
		Scope:    domain.Scope{Company: "acme", Area: "platform"},
		DailyCap: 2, Reason: "night-shift blocking behind structural guards",
		CreatedBy: "usr_ana", CreatedAt: day0,
		ExpiresAt: day0.Add(30 * 24 * time.Hour),
	}
	if over != nil {
		over(&g)
	}
	return g
}

// The mandate's own invariants: never a pattern, never unbounded, never
// without a reason or a plausible ceiling.
func TestValidate_refusesWhatAMandateMustNeverBe(t *testing.T) {
	t.Parallel()
	for name, over := range map[string]func(*standing.Grant){
		"glob tool":        func(g *standing.Grant) { g.Tool = "cloudflare.*.block_ip" },
		"empty tool":       func(g *standing.Grant) { g.Tool = "" },
		"no agent":         func(g *standing.Grant) { g.Agent = "" },
		"no scope":         func(g *standing.Grant) { g.Scope = domain.Scope{} },
		"zero cap":         func(g *standing.Grant) { g.DailyCap = 0 },
		"cap past ceiling": func(g *standing.Grant) { g.DailyCap = 101 },
		"no reason":        func(g *standing.Grant) { g.Reason = "  " },
		"already expired":  func(g *standing.Grant) { g.ExpiresAt = day0.Add(-time.Hour) },
		"past max ttl":     func(g *standing.Grant) { g.ExpiresAt = day0.Add(91 * 24 * time.Hour) },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if err := standing.Validate(grant(over), day0); err == nil {
				t.Fatal("Validate accepted it")
			}
		})
	}
	if err := standing.Validate(grant(nil), day0); err != nil {
		t.Fatalf("Validate refused the valid grant: %v", err)
	}
}

// Claim covers only the exact tool, agent and scope, consumes one use per
// call inside the ceiling, and refuses once today's ceiling is spent —
// atomically, so two runs cannot spend the same slot.
func TestClaim_coversExactlyAndSpendsTheCeiling(t *testing.T) {
	store, _ := storeFor(t)
	ctx := t.Context()
	if err := store.Create(ctx, grant(nil)); err != nil {
		t.Fatalf("Create: %v", err)
	}

	for name, args := range map[string]struct {
		tool  domain.ToolID
		agent domain.AgentID
		scope domain.Scope
	}{
		"another instance": {"cloudflare.other.block_ip", "sentinel", domain.Scope{Company: "acme", Area: "platform"}},
		"another agent":    {"cloudflare.edge.block_ip", "intruder", domain.Scope{Company: "acme", Area: "platform"}},
		"another area":     {"cloudflare.edge.block_ip", "sentinel", domain.Scope{Company: "acme", Area: "ops"}},
	} {
		if _, ok, err := store.Claim(ctx, args.tool, args.agent, args.scope, "run-x", day0.Add(time.Hour)); err != nil || ok {
			t.Fatalf("%s: covered=%v err=%v, want no cover", name, ok, err)
		}
	}

	scope := domain.Scope{Company: "acme", Area: "platform"}
	for i := range 2 {
		got, ok, err := store.Claim(ctx, "cloudflare.edge.block_ip", "sentinel", scope, "run-1", day0.Add(time.Hour))
		if err != nil || !ok {
			t.Fatalf("claim %d: ok=%v err=%v", i, ok, err)
		}
		if got.CreatedBy != "usr_ana" || got.ID != "SG-1" {
			t.Fatalf("claim %d: grant = %+v", i, got)
		}
	}
	if _, ok, err := store.Claim(ctx, "cloudflare.edge.block_ip", "sentinel", scope, "run-2", day0.Add(2*time.Hour)); err != nil || ok {
		t.Fatalf("past the ceiling: covered=%v err=%v, want refusal", ok, err)
	}
	// A new day opens a new ceiling.
	if _, ok, err := store.Claim(ctx, "cloudflare.edge.block_ip", "sentinel", scope, "run-3", day0.Add(26*time.Hour)); err != nil || !ok {
		t.Fatalf("next day: covered=%v err=%v, want cover", ok, err)
	}
}

// Expiry and revocation both stop coverage immediately.
func TestClaim_expiredOrRevoked_stopsCovering(t *testing.T) {
	store, _ := storeFor(t)
	ctx := t.Context()
	scope := domain.Scope{Company: "acme", Area: "platform"}
	if err := store.Create(ctx, grant(nil)); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, ok, _ := store.Claim(ctx, "cloudflare.edge.block_ip", "sentinel", scope, "r", day0.Add(31*24*time.Hour)); ok {
		t.Fatal("an expired grant covered")
	}
	if err := store.Revoke(ctx, "SG-1", "usr_ana", day0.Add(time.Hour)); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if _, ok, _ := store.Claim(ctx, "cloudflare.edge.block_ip", "sentinel", scope, "r", day0.Add(2*time.Hour)); ok {
		t.Fatal("a revoked grant covered")
	}
	if err := store.Revoke(ctx, "SG-1", "usr_ana", day0); err == nil {
		t.Fatal("revoking twice succeeded")
	}
	// The row survives revocation: the trail shows the mandate and its end.
	grants, err := store.List(ctx)
	if err != nil || len(grants) != 1 || grants[0].Status != standing.StatusRevoked {
		t.Fatalf("List = %+v, %v", grants, err)
	}
}
