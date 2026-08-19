package scope_test

import (
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/scope"
)

// The registry exists so two spellings of one area cannot both exist. These
// are the properties that make that true past the point where a person types.

func storeFor(t *testing.T) *scope.Store {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		if os.Getenv("REQUIRE_DATABASE") != "" {
			t.Fatal("REQUIRE_DATABASE is set but TEST_DATABASE_URL is empty")
		}
		t.Skip("TEST_DATABASE_URL is unset; skipping the scope suite")
	}

	pool, err := pgxpool.New(t.Context(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := ledger.Migrate(t.Context(), pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if _, err := pool.Exec(t.Context(), `delete from scopes where company_id in ('suite', 'suite-other')`); err != nil {
		t.Fatalf("clean: %v", err)
	}
	// The company has to exist before an area can be registered in it, which
	// is the whole point of the key added in 0029. Seeding it here is what a
	// real installation does through the administration area.
	if _, err := pool.Exec(t.Context(), `
		insert into companies (company_id, name)
		values ('suite', 'suite'), ('suite-other', 'suite-other')
		on conflict (company_id) do nothing`); err != nil {
		t.Fatalf("seed company: %v", err)
	}
	return scope.NewStore(pool)
}

func TestPut_twoSpellingsOfOneArea_becomeOneRow(t *testing.T) {
	store := storeFor(t)
	ctx := t.Context()

	if _, err := store.Put(ctx, "suite", "Risco de Crédito", "Risco", "usr_a"); err != nil {
		t.Fatalf("first: %v", err)
	}
	if _, err := store.Put(ctx, "suite", "risco de credito", "Risco de crédito", "usr_b"); err != nil {
		t.Fatalf("second: %v", err)
	}

	got, err := store.List(ctx, []domain.Scope{{Company: "suite"}})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d areas, want 1: %+v", len(got), got)
	}
	// The label is the writer's, and the last writer wins. The id is the
	// platform's, and nobody's typing changes it.
	if got[0].Scope.Area != "risco-de-credito" || got[0].Label != "Risco de crédito" {
		t.Errorf("got %+v", got[0])
	}
}

func TestList_scopeTheCallerCannotReach_isNotReturned(t *testing.T) {
	store := storeFor(t)
	ctx := t.Context()

	if _, err := store.Put(ctx, "suite", "cx", "", "usr_a"); err != nil {
		t.Fatalf("put: %v", err)
	}

	got, err := store.List(ctx, []domain.Scope{{Company: "outra"}})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %+v, want nothing", got)
	}
}

func TestList_grantOnOneArea_seesThatAreaAndNoOther(t *testing.T) {
	store := storeFor(t)
	ctx := t.Context()

	for _, area := range []string{"cx", "financeiro"} {
		if _, err := store.Put(ctx, "suite", area, "", "usr_a"); err != nil {
			t.Fatalf("put %s: %v", area, err)
		}
	}

	got, err := store.List(ctx, []domain.Scope{{Company: "suite", Area: "cx"}})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 1 || got[0].Scope.Area != "cx" {
		t.Errorf("got %+v, want cx alone", got)
	}
}

func TestList_grantOnTheCompanyAndOnAnAreaInIt_returnsTheAreaOnce(t *testing.T) {
	store := storeFor(t)
	ctx := t.Context()

	if _, err := store.Put(ctx, "suite", "cx", "", "usr_a"); err != nil {
		t.Fatalf("put: %v", err)
	}

	// The shape a real administrator has: curator over the company, and a
	// narrower grant on one area inside it. Both reach the same row.
	got, err := store.List(ctx, []domain.Scope{
		{Company: "suite"},
		{Company: "suite", Area: "cx"},
	})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("got %d rows, want cx once: %+v", len(got), got)
	}
}

func TestList_installationGrantSeesEveryCompanyArea(t *testing.T) {
	store := storeFor(t)
	ctx := t.Context()

	if _, err := store.Put(ctx, "suite", "cx", "", "usr_a"); err != nil {
		t.Fatalf("put suite: %v", err)
	}
	if _, err := store.Put(ctx, "suite-other", "financeiro", "", "usr_a"); err != nil {
		t.Fatalf("put suite-other: %v", err)
	}

	got, err := store.List(ctx, []domain.Scope{{Company: domain.Installation}})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	seen := map[domain.Scope]bool{}
	for _, area := range got {
		seen[area.Scope] = true
	}
	for _, want := range []domain.Scope{
		{Company: "suite", Area: "cx"},
		{Company: "suite-other", Area: "financeiro"},
	} {
		if !seen[want] {
			t.Fatalf("got %+v, want %s", got, want)
		}
	}
}
