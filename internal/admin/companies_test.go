package admin_test

import (
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/scope"
)

/*
Creating a company.

The property worth the whole file: a company created without a grant in it is
invisible to whoever created it. Every listing here is filtered by the scopes a
caller holds, so the screen reports success and then shows nothing — which is
the defect this platform already had one level down, where registering an area
in an unknown company saved cleanly and vanished.

So the grant is not something the caller remembers to do afterwards. It is in
the same transaction, and this is the test that keeps it there.
*/
func TestCreate_grantsTheCreator_soTheCompanyIsNotInvisible(t *testing.T) {
	companies, pool := companiesFor(t)

	if _, err := companies.Create(t.Context(), "acme-two", "Acme Dois", "usr_ana"); err != nil {
		t.Fatalf("create: %v", err)
	}

	var held int
	if err := pool.QueryRow(t.Context(), `
		select count(*) from role_grants
		where principal_id = 'usr_ana' and company_id = 'acme-two' and area_id = ''`,
	).Scan(&held); err != nil {
		t.Fatalf("read grants: %v", err)
	}
	if held != 1 {
		t.Fatal("the company was created and its creator was granted nothing in it")
	}
}

// And the area that follows can actually be registered in it — the foreign key
// added in 0029 means a company that does not exist is an area that cannot.
func TestCreate_thenAnAreaInIt_isAccepted(t *testing.T) {
	companies, pool := companiesFor(t)

	if _, err := companies.Create(t.Context(), "acme-two", "", "usr_ana"); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := scope.NewStore(pool).Put(
		t.Context(), "acme-two", "Risco", "Risco", "usr_ana"); err != nil {
		t.Fatalf("register an area in a company that exists: %v", err)
	}
}

func TestCreate_twice_refusesTheSecond(t *testing.T) {
	companies, _ := companiesFor(t)

	if _, err := companies.Create(t.Context(), "acme-two", "", "usr_ana"); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := companies.Create(t.Context(), "acme-two", "", "usr_bob"); err == nil {
		t.Fatal("a second company took an identifier that was already taken")
	}
}

// The identifier reaches URLs, settings keys and every scope written as
// "company/area". One that has to be escaped to be addressed is one somebody
// will address wrongly.
func TestCreate_identifierThatCannotBeAddressed_isRefused(t *testing.T) {
	companies, _ := companiesFor(t)

	for _, bad := range []string{"", "Acme SA", "acme/two", "*", "-acme"} {
		if _, err := companies.Create(t.Context(), domain.CompanyID(bad), "", "usr_ana"); err == nil {
			t.Errorf("accepted %q as an identifier", bad)
		}
	}
}

/*
Archiving withdraws a company; it never deletes one.

Its runs, decisions and trail name it, and removing the row would leave every
one of them pointing at nothing. A record that cannot say which company an act
belonged to is not the record this platform claims to keep.
*/
func TestArchive_leavesTheCompanyReadable(t *testing.T) {
	companies, _ := companiesFor(t)

	if _, err := companies.Create(t.Context(), "acme-two", "Acme Dois", "usr_ana"); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := companies.Archive(t.Context(), "acme-two", "usr_ana"); err != nil {
		t.Fatalf("archive: %v", err)
	}

	listed, err := companies.List(t.Context())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, company := range listed {
		if company.ID == "acme-two" {
			if !company.Archived {
				t.Error("an archived company does not say so")
			}
			return
		}
	}
	t.Fatal("an archived company disappeared from the listing")
}

func companiesFor(t *testing.T) (*admin.Companies, *pgxpool.Pool) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		if os.Getenv("REQUIRE_DATABASE") != "" {
			t.Fatal("REQUIRE_DATABASE is set but TEST_DATABASE_URL is empty")
		}
		t.Skip("TEST_DATABASE_URL is unset; skipping the companies suite")
	}

	pool, err := pgxpool.New(t.Context(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := ledger.Migrate(t.Context(), pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if _, err := pool.Exec(t.Context(), `
		delete from scopes where company_id = 'acme-two';
		delete from role_grants where company_id = 'acme-two';
		delete from companies where company_id = 'acme-two'`); err != nil {
		t.Fatalf("clean: %v", err)
	}
	// A grant references a principal, so the creator has to be somebody. In
	// production they always are: `by` comes from the authenticated caller.
	if _, err := pool.Exec(t.Context(), `
		insert into principals (principal_id, kind, display, subject)
		values ('usr_ana', 'user', 'Ana', 'ana'), ('usr_bob', 'user', 'Bob', 'bob')
		on conflict (principal_id) do nothing`); err != nil {
		t.Fatalf("seed principals: %v", err)
	}
	return admin.NewCompanies(pool), pool
}
