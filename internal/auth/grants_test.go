package auth_test

import (
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/ledger"
)

// Two things write grants: an identity provider, on every sign-in, and an
// operator, by hand. They must not overwrite each other — a grant somebody set
// deliberately that disappears the next time its holder signs in is a console
// offering an edit that silently reverts.

func directoryFor(t *testing.T) (*auth.Postgres, *pgxpool.Pool) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is unset; skipping the grants suite")
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
		`truncate role_grants, principals, sessions cascade`); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	return auth.NewPostgres(pool), pool
}

func personIn(t *testing.T, dir *auth.Postgres, subject string) string {
	t.Helper()
	id, err := dir.UpsertPrincipal(t.Context(), "keycloak", subject, subject, subject+"@example.com")
	if err != nil {
		t.Fatalf("UpsertPrincipal: %v", err)
	}
	return id
}

func rolesOf(t *testing.T, dir *auth.Postgres, principalID string) []domain.HeldGrant {
	t.Helper()
	people, err := dir.People(t.Context())
	if err != nil {
		t.Fatalf("People: %v", err)
	}
	for _, p := range people {
		if p.ID == principalID {
			return p.Grants
		}
	}
	t.Fatalf("principal %s is not in the listing", principalID)
	return nil
}

func grant(company, area string, role domain.Role) domain.Grant {
	return domain.Grant{
		Scope: domain.Scope{Company: domain.CompanyID(company), Area: domain.AreaID(area)},
		Role:  role,
	}
}

func TestSetGrants_survivesTheNextSignIn(t *testing.T) {
	dir, _ := directoryFor(t)
	person := personIn(t, dir, "ana")

	// An operator grants directly — a service account, or somebody the
	// provider's groups do not cover.
	if err := dir.SetGrants(t.Context(), person,
		[]domain.Grant{grant("acme", "cx", domain.RoleApprover)}, "usr_operator"); err != nil {
		t.Fatalf("SetGrants: %v", err)
	}

	// Then they sign in, and the provider asserts something else entirely.
	if err := dir.ReplaceAssertedGrants(t.Context(), person, "keycloak",
		[]domain.Grant{grant("acme", "cx", domain.RoleAuthor)}); err != nil {
		t.Fatalf("ReplaceAssertedGrants: %v", err)
	}

	held := rolesOf(t, dir, person)
	if len(held) != 2 {
		t.Fatalf("grants = %+v, want both the granted and the asserted one", held)
	}
}

func TestReplaceAssertedGrants_dropsWhatTheProviderNoLongerAsserts(t *testing.T) {
	dir, _ := directoryFor(t)
	person := personIn(t, dir, "bruno")

	if err := dir.ReplaceAssertedGrants(t.Context(), person, "keycloak",
		[]domain.Grant{grant("acme", "cx", domain.RoleAuthor)}); err != nil {
		t.Fatalf("first sign-in: %v", err)
	}
	// Removed from the group. The next sign-in must take the role with it, or
	// leaving a team would leave the access behind.
	if err := dir.ReplaceAssertedGrants(t.Context(), person, "keycloak", nil); err != nil {
		t.Fatalf("second sign-in: %v", err)
	}

	if held := rolesOf(t, dir, person); len(held) != 0 {
		t.Errorf("grants = %+v, want none", held)
	}
}

func TestReplaceAssertedGrants_leavesAnotherProvidersAssertionsAlone(t *testing.T) {
	dir, _ := directoryFor(t)
	person := personIn(t, dir, "carla")

	if err := dir.ReplaceAssertedGrants(t.Context(), person, "keycloak",
		[]domain.Grant{grant("acme", "cx", domain.RoleAuthor)}); err != nil {
		t.Fatalf("keycloak: %v", err)
	}
	// Two providers is normal — staff in one, partners in another. Signing in
	// through one must not revoke what the other asserted.
	if err := dir.ReplaceAssertedGrants(t.Context(), person, "entra",
		[]domain.Grant{grant("acme", "financeiro", domain.RoleAuditor)}); err != nil {
		t.Fatalf("entra: %v", err)
	}

	if held := rolesOf(t, dir, person); len(held) != 2 {
		t.Errorf("grants = %+v, want one from each provider", held)
	}
}

func TestSetGrants_replacesOnlyWhatWasSetByHand(t *testing.T) {
	dir, _ := directoryFor(t)
	person := personIn(t, dir, "dora")

	if err := dir.ReplaceAssertedGrants(t.Context(), person, "keycloak",
		[]domain.Grant{grant("acme", "cx", domain.RoleAuthor)}); err != nil {
		t.Fatalf("sign-in: %v", err)
	}
	if err := dir.SetGrants(t.Context(), person,
		[]domain.Grant{grant("acme", "cx", domain.RoleApprover)}, "usr_operator"); err != nil {
		t.Fatalf("first grant: %v", err)
	}
	// Taking the manual one away must not take the asserted one with it: the
	// operator never granted that, and cannot revoke it here — the group does.
	if err := dir.SetGrants(t.Context(), person, nil, "usr_operator"); err != nil {
		t.Fatalf("revoke: %v", err)
	}

	held := rolesOf(t, dir, person)
	if len(held) != 1 || held[0].Role != domain.RoleAuthor {
		t.Errorf("grants = %+v, want only what the provider asserts", held)
	}
}

func TestPeople_saysWhereEachGrantCameFrom(t *testing.T) {
	dir, _ := directoryFor(t)
	person := personIn(t, dir, "elias")

	if err := dir.ReplaceAssertedGrants(t.Context(), person, "keycloak",
		[]domain.Grant{grant("acme", "cx", domain.RoleAuthor)}); err != nil {
		t.Fatalf("sign-in: %v", err)
	}

	people, err := dir.People(t.Context())
	if err != nil {
		t.Fatalf("People: %v", err)
	}
	if len(people) != 1 || people[0].Display != "elias" {
		t.Fatalf("people = %+v", people)
	}
	// The screen has to say which grants it may edit. One the provider
	// asserts is not one an operator can take away here, and offering the
	// button would be offering a revocation the next sign-in undoes.
	if !people[0].Grants[0].Asserted {
		t.Error("an asserted grant is not marked as one")
	}
}
