package egress_test

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/egress"
	"github.com/fuseone/agents/internal/egressmetrics"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/testdb"
)

func TestRecordDenial_feedsRuntimeHealthWithBoundedCodes(t *testing.T) {
	store, pool := egressStore(t)
	now := time.Date(2026, 8, 24, 10, 0, 0, 0, time.UTC)

	denials := []egress.Denial{{
		Server: "crm", Host: "blocked.internal", Port: 443,
		Code: egressmetrics.CodeDestinationDenied, FirstSeen: now, LastSeen: now,
	}, {
		Server: "crm", Host: "blocked.internal", Port: 443,
		Code: egressmetrics.CodeDestinationDenied, FirstSeen: now.Add(time.Minute), LastSeen: now.Add(time.Minute),
	}, {
		Server: "github", Host: "jira-prod.transition_ACME-4417", Port: 443,
		Code: "github-mcp.create_issue", FirstSeen: now.Add(2 * time.Minute), LastSeen: now.Add(2 * time.Minute),
	}}
	for _, denial := range denials {
		if err := store.RecordDenial(t.Context(), denial); err != nil {
			t.Fatalf("RecordDenial: %v", err)
		}
	}

	empty, err := ledger.NewPostgres(pool).RuntimeHealth(t.Context(), domain.RunFilter{
		Since: now.Add(-time.Minute),
	})
	if err != nil {
		t.Fatalf("RuntimeHealth empty filter: %v", err)
	}
	if len(empty.EgressDenials) != 0 {
		t.Fatalf("empty filter egress denials = %+v, want no global signal", empty.EgressDenials)
	}

	health, err := ledger.NewPostgres(pool).RuntimeHealth(t.Context(), domain.RunFilter{
		Scopes: []domain.Scope{{Company: domain.Installation}}, Since: now.Add(-time.Minute),
	})
	if err != nil {
		t.Fatalf("RuntimeHealth: %v", err)
	}
	byCode := map[string]domain.RuntimeEgressDenialBucket{}
	for _, bucket := range health.EgressDenials {
		byCode[bucket.Code] = bucket
	}
	denied := byCode[egressmetrics.CodeDestinationDenied]
	if denied.Attempts != 2 || denied.Servers != 1 || denied.Destinations != 1 {
		t.Fatalf("destination denied bucket = %+v, want retries collapsed by target", denied)
	}
	other := byCode[egressmetrics.CodeOther]
	if other.Attempts != 1 || other.Servers != 1 || other.Destinations != 1 {
		t.Fatalf("dynamic code bucket = %+v, want bounded other", other)
	}

	scoped, err := ledger.NewPostgres(pool).RuntimeHealth(t.Context(), domain.RunFilter{
		Scopes: []domain.Scope{{Company: "acme", Area: "ops"}}, Since: now.Add(-time.Minute),
	})
	if err != nil {
		t.Fatalf("RuntimeHealth scoped: %v", err)
	}
	if len(scoped.EgressDenials) != 0 {
		t.Fatalf("scoped egress denials = %+v, want no global signal", scoped.EgressDenials)
	}
}

func egressStore(t *testing.T) (*egress.Postgres, *pgxpool.Pool) {
	t.Helper()
	pool := testdb.Pool(t)
	if _, err := pool.Exec(t.Context(), `truncate mcp_egress_denials, run_steps, runs`); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	return egress.NewPostgres(pool), pool
}
