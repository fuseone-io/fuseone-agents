package standing_test

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/spec"
	"github.com/fuseone/agents/internal/standing"
)

// publishAgent writes one version of an agent the way the registry does; the
// standing line is appended to the frontmatter when given.
func publishAgent(t *testing.T, pool *pgxpool.Pool, agent, standingLine, body string) {
	t.Helper()
	file := fmt.Sprintf(`---
id: %s
name: Factory role
area: platform
provider: openai
model: test-model
tools: [cloudflare.edge.block_ip]
budget: {micros: 500000, steps: 60}
%s---

%s
`, agent, standingLine, body)
	s, err := spec.Parse("test.agent.md", []byte(file))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if err := spec.NewRegistry(pool).Publish(t.Context(), s, "usr_ana", "acme"); err != nil {
		t.Fatalf("Publish: %v", err)
	}
}

func countGrants(t *testing.T, pool *pgxpool.Pool, agent string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(t.Context(),
		`select count(*) from standing_approvals where agent_id = $1`, agent).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	return n
}

// A grant for an agent whose current version forbids standing approvals is
// refused at creation, and nothing is written (#192). The version that
// counts is the current one: an older version that allowed them does not.
func TestCreate_agentWhoseCurrentVersionForbids_refused(t *testing.T) {
	store, pool := storeFor(t)
	agent := fmt.Sprintf("factory-dev-%d", time.Now().UnixNano())
	publishAgent(t, pool, agent, "", "Allowed once.")
	publishAgent(t, pool, agent, "standing_approvals: forbidden\n", "Forbidden now.")

	err := store.Create(t.Context(), grant(func(g *standing.Grant) { g.Agent = domain.AgentID(agent) }))
	if !errors.Is(err, standing.ErrStandingForbidden) {
		t.Fatalf("Create = %v, want ErrStandingForbidden", err)
	}
	if n := countGrants(t, pool, agent); n != 0 {
		t.Fatalf("grants = %d, want none written", n)
	}
}

// The other direction: forbidden once, allowed now, and an agent nobody has
// published. Both keep today's behaviour.
func TestCreate_agentWhoseCurrentVersionAllows_granted(t *testing.T) {
	store, pool := storeFor(t)
	agent := fmt.Sprintf("support-%d", time.Now().UnixNano())
	publishAgent(t, pool, agent, "standing_approvals: forbidden\n", "Forbidden once.")
	publishAgent(t, pool, agent, "", "Allowed now.")

	for i, id := range []string{agent, "never-published"} {
		g := grant(func(g *standing.Grant) {
			g.ID, g.Agent = fmt.Sprintf("SG-%d", i), domain.AgentID(id)
		})
		if err := store.Create(t.Context(), g); err != nil {
			t.Fatalf("Create for %s: %v", id, err)
		}
	}
	if n := countGrants(t, pool, agent); n != 1 {
		t.Fatalf("grants = %d, want the one created", n)
	}
}
