package memory

import (
	"strings"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
)

func TestFindWhereKeepsSearchScoreOutOfThePredicate(t *testing.T) {
	t.Parallel()
	where, args, order := findWhere(domain.MemoryQuery{
		Scope: domain.Scope{Company: "acme", Area: "ops"}, AgentID: "triage",
		Search: "superset alerta slack 500",
		Now:    time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC),
	})
	if len(args) != 8 {
		t.Fatalf("args = %d, want scope, agent, four search terms, now", len(args))
	}
	if strings.Contains(where, "case when") {
		t.Fatalf("where = %s, want plain boolean search predicates", where)
	}
	if !strings.Contains(where, " ilike ") {
		t.Fatalf("where = %s, want indexable ilike predicates", where)
	}
	if !strings.Contains(order, "case when") {
		t.Fatalf("order = %s, want scoring only in order by", order)
	}
}
