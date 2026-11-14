package auth_test

import (
	"errors"
	"testing"

	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/testdb"
)

/*
Somebody must always be able to manage identity over the whole installation.

Whatever role carries identity:write at the installation scope, held by an
active person, from a provider or from this console. Both doors that take it
away — removing a person and replacing their grants — refuse a change that
would leave nobody, and judge it on the state the change produces.
*/

func installation(role domain.Role) []domain.Grant {
	return []domain.Grant{grant(string(domain.Installation), "", role)}
}

// soleHolders is the fixture with its provider administrator gone, and the
// given local people holding the given installation role instead.
func soleHolders(t *testing.T, r removal, role domain.Role, usernames ...string) []string {
	t.Helper()
	ids := make([]string, 0, len(usernames))
	for _, u := range usernames {
		id := r.local(t, u)
		if err := r.dir.SetGrants(t.Context(), id, installation(role), r.admin); err != nil {
			t.Fatalf("SetGrants: %v", err)
		}
		ids = append(ids, id)
	}
	if err := r.dir.SetGrants(t.Context(), r.admin, nil, ids[0]); err != nil {
		t.Fatalf("take the provider administrator's grant: %v", err)
	}
	return ids
}

func TestRemoveLocal_theLastInstallationCurator_isRefused(t *testing.T) {
	r := removalFixture(t)
	ids := soleHolders(t, r, domain.RoleCurator, "cura")
	if err := r.remove(t, ids[0], "usr_operator"); !errors.Is(err, auth.ErrLastAdministrator) {
		t.Fatalf("err = %v, want ErrLastAdministrator", err)
	}
}

// A provider's grant counts like one made here: it is somebody who can get in.
func TestRemoveLocal_aProviderAssertedAdministratorBeside_isEnough(t *testing.T) {
	r := removalFixture(t)
	ana := r.local(t, "ana")
	installationAdmin(t, r.dir, ana)
	if err := r.dir.SetGrants(t.Context(), r.admin, nil, "test"); err != nil {
		t.Fatalf("SetGrants: %v", err)
	}
	if err := r.dir.ReplaceAssertedGrants(t.Context(), r.admin, "keycloak", installation(domain.RoleAdmin)); err != nil {
		t.Fatalf("ReplaceAssertedGrants: %v", err)
	}
	if err := r.remove(t, ana, r.admin); err != nil {
		t.Fatalf("RemoveLocal: %v", err)
	}
}

func TestRemoveLocal_twoCuratorsRemovingEachOther_oneIsRefused(t *testing.T) {
	r := removalFixture(t)
	ids := soleHolders(t, r, domain.RoleCurator, "cura", "curb")
	contention := testdb.NewContention(t, 2)
	dir := auth.NewPostgres(contention.Pool)
	remove := func(target, by string) func() error {
		return func() error {
			return dir.RemoveLocal(t.Context(), auth.Removal{PrincipalID: target, By: by, At: removedAt})
		}
	}

	errs := contention.Overlap(t,
		`select 1 from principals where principal_id = any($1) for update`, []any{ids},
		[]func() error{remove(ids[0], ids[1]), remove(ids[1], ids[0])})

	assertOneRefused(t, errs)
}

func TestSetGrants_theOnlyAdministratorDroppingTheirOwn_isRefusedAndKeepsIt(t *testing.T) {
	r := removalFixture(t)
	ids := soleHolders(t, r, domain.RoleAdmin, "ana")

	err := r.dir.SetGrants(t.Context(), ids[0], []domain.Grant{grant("default", "", domain.RoleAuthor)}, ids[0])
	if !errors.Is(err, auth.ErrLastAdministrator) {
		t.Fatalf("err = %v, want ErrLastAdministrator", err)
	}
	if held := r.scalar(t, `select count(*) from role_grants where principal_id = $1 and company_id = '*'`, ids[0]); held != 1 {
		t.Errorf("the refused change took the grant anyway: %d left", held)
	}
}

// Moving from one role that manages identity to another leaves somebody.
func TestSetGrants_theOnlyAdministratorBecomingCurator_isAllowed(t *testing.T) {
	r := removalFixture(t)
	ids := soleHolders(t, r, domain.RoleAdmin, "ana")
	if err := r.dir.SetGrants(t.Context(), ids[0], installation(domain.RoleCurator), ids[0]); err != nil {
		t.Fatalf("SetGrants: %v", err)
	}
}

// Before anybody holds it there is nothing to protect: the first grant must go in.
func TestSetGrants_anInstallationWithNobodyYet_acceptsAnyChange(t *testing.T) {
	dir, _ := directoryFor(t)
	bia := personIn(t, dir, "bia")
	if err := dir.SetGrants(t.Context(), bia, []domain.Grant{grant("acme", "cx", domain.RoleAuthor)}, "test"); err != nil {
		t.Fatalf("SetGrants: %v", err)
	}
}

func TestSetGrants_twoAdministratorsEachDroppingTheirOwn_oneIsRefused(t *testing.T) {
	r := removalFixture(t)
	ids := soleHolders(t, r, domain.RoleAdmin, "ana", "bia")
	contention := testdb.NewContention(t, 2)
	dir := auth.NewPostgres(contention.Pool)
	drop := func(id string) func() error {
		return func() error { return dir.SetGrants(t.Context(), id, nil, id) }
	}

	errs := contention.Overlap(t,
		`select 1 from principals where principal_id = any($1) for update`, []any{ids},
		[]func() error{drop(ids[0]), drop(ids[1])})

	assertOneRefused(t, errs)
}

func assertOneRefused(t *testing.T, errs []error) {
	t.Helper()
	refused := 0
	for _, err := range errs {
		switch {
		case errors.Is(err, auth.ErrLastAdministrator):
			refused++
		case err != nil:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if refused != 1 {
		t.Fatalf("errors = %v, want exactly one refused as the last administrator", errs)
	}
}

func TestSetGrants_aRemovedPerson_isRefused(t *testing.T) {
	r := removalFixture(t)
	ana := r.local(t, "ana")
	if err := r.remove(t, ana, r.admin); err != nil {
		t.Fatalf("RemoveLocal: %v", err)
	}

	err := r.dir.SetGrants(t.Context(), ana, []domain.Grant{grant("acme", "cx", domain.RoleAuthor)}, r.admin)
	if !errors.Is(err, auth.ErrRemoved) {
		t.Fatalf("err = %v, want ErrRemoved", err)
	}
	if held := r.scalar(t, `select count(*) from role_grants where principal_id = $1`, ana); held != 0 {
		t.Errorf("a removed person holds %d grants", held)
	}
}

// A disabled machine keeps its grants editable, as it always has: disabling
// one is reversible, and its grants are part of what comes back.
func TestSetGrants_aDisabledServiceAccount_stillTakesGrants(t *testing.T) {
	r := removalFixture(t)
	svc, err := r.dir.CreateServiceAccount(t.Context(), "publisher", "", r.admin)
	if err != nil {
		t.Fatalf("CreateServiceAccount: %v", err)
	}
	if err := r.dir.SetServiceAccountDisabled(t.Context(), svc.ID, true, r.admin, removedAt); err != nil {
		t.Fatalf("disable: %v", err)
	}
	if err := r.dir.SetGrants(t.Context(), svc.ID, []domain.Grant{grant("acme", "cx", domain.RoleAuthor)}, r.admin); err != nil {
		t.Fatalf("SetGrants: %v", err)
	}
}

func TestRemoveLocal_somebodyAlreadyRemoved_isRefusedAsRemoved(t *testing.T) {
	r := removalFixture(t)
	ana := r.local(t, "ana")
	if err := r.remove(t, ana, r.admin); err != nil {
		t.Fatalf("RemoveLocal: %v", err)
	}
	if err := r.remove(t, ana, r.admin); !errors.Is(err, auth.ErrRemoved) {
		t.Fatalf("err = %v, want ErrRemoved", err)
	}
}

/*
A grant and a removal of the same person at the same moment. Whichever goes
first, the person ends removed and holding nothing: a grant written after the
removal would sit hidden on an account nobody sees in the list.
*/
func TestSetGrants_racingARemoval_leavesTheRemovedHoldingNothing(t *testing.T) {
	r := removalFixture(t)
	ana := r.local(t, "ana")
	contention := testdb.NewContention(t, 2)
	dir := auth.NewPostgres(contention.Pool)

	errs := contention.Overlap(t,
		`select 1 from principals where principal_id = $1 for update`, []any{ana},
		[]func() error{
			func() error {
				return dir.SetGrants(t.Context(), ana, []domain.Grant{grant("acme", "cx", domain.RoleAuthor)}, r.admin)
			},
			func() error {
				return dir.RemoveLocal(t.Context(), auth.Removal{PrincipalID: ana, By: r.admin, At: removedAt})
			},
		})

	if errs[1] != nil || (errs[0] != nil && !errors.Is(errs[0], auth.ErrRemoved)) {
		t.Fatalf("errors = %v", errs)
	}
	if held := r.scalar(t, `select count(*) from role_grants where principal_id = $1`, ana); held != 0 {
		t.Errorf("the removed person holds %d grants", held)
	}
}

// A machine holding an installation role does not count: no machine manages
// identity whatever it holds, so it is nobody who could repair anything.
func TestSetGrants_aMachineBesideTheOnlyAdministrator_doesNotCount(t *testing.T) {
	r := removalFixture(t)
	ids := soleHolders(t, r, domain.RoleAdmin, "ana")
	svc, err := r.dir.CreateServiceAccount(t.Context(), "publisher", "", r.admin)
	if err != nil {
		t.Fatalf("CreateServiceAccount: %v", err)
	}
	if err := r.dir.SetGrants(t.Context(), svc.ID, installation(domain.RoleCurator), ids[0]); err != nil {
		t.Fatalf("SetGrants: %v", err)
	}

	if err := r.dir.SetGrants(t.Context(), ids[0], nil, ids[0]); !errors.Is(err, auth.ErrLastAdministrator) {
		t.Fatalf("err = %v, want ErrLastAdministrator", err)
	}
}
