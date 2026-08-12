package ledger_test

import (
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/ledger"
)

// A simulation is a real run in the ledger, so the trail and the diagram read
// it unchanged. The price is that every projection has to exclude it, and a
// simulated run counted as production is a wrong number somebody acts on.

func TestList_excludesSimulatedRuns(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is unset")
	}
	pool, err := pgxpool.New(t.Context(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := ledger.Migrate(t.Context(), pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if _, err := pool.Exec(t.Context(), `delete from runs where run_id like 'sim_%'`); err != nil {
		t.Fatalf("clean: %v", err)
	}
	if _, err := pool.Exec(t.Context(), `
		insert into runs (run_id, company_id, area_id, agent_id, version_id, on_behalf_of,
		                  phase, last_seq, started_at, updated_at, simulated)
		values ('sim_real', 'c', 'a', 'ag', 'v', '', 'finished', 1, now(), now(), false),
		       ('sim_dry',  'c', 'a', 'ag', 'v', '', 'finished', 1, now(), now(), true)`); err != nil {
		t.Fatalf("seed: %v", err)
	}

	runs, err := ledger.NewPostgres(pool).ListRuns(t.Context(),
		domain.RunFilter{Scope: domain.Scope{Company: "c", Area: "a"}}, "", 50)
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	for _, run := range runs {
		if run.RunID == "sim_dry" {
			t.Fatal("a simulated run reached the runs list")
		}
	}
}

func TestClaim_neverPicksUpASimulatedRun(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is unset")
	}
	pool, err := pgxpool.New(t.Context(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := ledger.Migrate(t.Context(), pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if _, err := pool.Exec(t.Context(), `delete from runs where run_id like 'sim_%'`); err != nil {
		t.Fatalf("clean: %v", err)
	}
	if _, err := pool.Exec(t.Context(), `
		insert into runs (run_id, company_id, area_id, agent_id, version_id, on_behalf_of,
		                  phase, last_seq, started_at, updated_at, simulated, next_attempt_at)
		values ('sim_dry', 'c', 'a', 'ag', 'v', '', 'running', 1, now(), now(), true, now())`); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// The one that matters most. A worker claiming a simulated run would
	// execute its proposals through the real tool layer, against real
	// systems, on the strength of a case somebody uploaded to find out what
	// would happen.
	claimed, err := ledger.NewPostgres(pool).Claim(t.Context(), "worker-1", time.Minute)
	if err == nil && claimed.RunID == "sim_dry" {
		t.Fatal("a worker claimed a simulated run")
	}
}
