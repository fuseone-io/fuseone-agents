package auth_test

import (
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/testdb"
)

/*
Removing somebody who signs in with a password.

Deactivated and taken off the list, never deleted: the trail and every run
name them by id, and an id that resolves to nothing is an audit that cannot be
read. What has to stop is everything they could still do.
*/

var removedAt = time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)

type removal struct {
	dir   *auth.Postgres
	pool  *pgxpool.Pool
	admin string
}

// removalFixture is an installation with one administrator, who signs in
// through a provider, and nobody else yet.
func removalFixture(t *testing.T) removal {
	t.Helper()
	dir, pool := directoryFor(t)
	if _, err := pool.Exec(t.Context(), `truncate admin_events`); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	admin := personIn(t, dir, "admin")
	installationAdmin(t, dir, admin)
	return removal{dir: dir, pool: pool, admin: admin}
}

func installationAdmin(t *testing.T, dir *auth.Postgres, principalID string) {
	t.Helper()
	if err := dir.SetGrants(t.Context(), principalID,
		[]domain.Grant{grant(string(domain.Installation), "", domain.RoleAdmin)}, "test"); err != nil {
		t.Fatalf("SetGrants: %v", err)
	}
}

func (r removal) local(t *testing.T, username string) string {
	t.Helper()
	id, err := auth.NewLocal(r.pool, r.dir).Create(t.Context(), username, "", "", "uma senha bem comprida", r.admin)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	return id
}

func (r removal) remove(t *testing.T, principalID, by string) error {
	t.Helper()
	return r.dir.RemoveLocal(t.Context(), auth.Removal{PrincipalID: principalID, By: by, At: removedAt})
}

func (r removal) scalar(t *testing.T, query string, args ...any) int {
	t.Helper()
	var n int
	if err := r.pool.QueryRow(t.Context(), query, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return n
}

func TestRemoveLocal_disablesAtTheGivenInstantAndKeepsTheRow(t *testing.T) {
	r := removalFixture(t)
	ana := r.local(t, "ana")

	if err := r.remove(t, ana, r.admin); err != nil {
		t.Fatalf("RemoveLocal: %v", err)
	}

	var disabled *time.Time
	if err := r.pool.QueryRow(t.Context(),
		`select disabled_at from principals where principal_id = $1`, ana).Scan(&disabled); err != nil {
		t.Fatalf("the row is gone: %v", err)
	}
	if disabled == nil || !disabled.Equal(removedAt) {
		t.Errorf("disabled_at = %v, want %v", disabled, removedAt)
	}
}

func TestRemoveLocal_revokesEverySessionTheyHold(t *testing.T) {
	r := removalFixture(t)
	ana := r.local(t, "ana")
	for i := range 2 {
		opened := removedAt.Add(-time.Duration(i+1) * time.Hour)
		if _, _, err := r.dir.CreateSession(t.Context(), ana, "test", "127.0.0.1", opened); err != nil {
			t.Fatalf("CreateSession: %v", err)
		}
	}

	if err := r.remove(t, ana, r.admin); err != nil {
		t.Fatalf("RemoveLocal: %v", err)
	}

	live := r.scalar(t, `select count(*) from sessions where principal_id = $1 and revoked_at is null`, ana)
	if live != 0 {
		t.Errorf("%d sessions still live", live)
	}
}

func TestRemoveLocal_takesAwayEveryGrant(t *testing.T) {
	r := removalFixture(t)
	ana := r.local(t, "ana")
	installationAdmin(t, r.dir, ana)

	if err := r.remove(t, ana, r.admin); err != nil {
		t.Fatalf("RemoveLocal: %v", err)
	}

	if held := r.scalar(t, `select count(*) from role_grants where principal_id = $1`, ana); held != 0 {
		t.Errorf("%d grants left", held)
	}
}

func TestRemoveLocal_leavesThemOffTheListing(t *testing.T) {
	r := removalFixture(t)
	ana := r.local(t, "ana")

	if err := r.remove(t, ana, r.admin); err != nil {
		t.Fatalf("RemoveLocal: %v", err)
	}

	people, err := r.dir.People(t.Context())
	if err != nil {
		t.Fatalf("People: %v", err)
	}
	for _, p := range people {
		if p.ID == ana {
			t.Fatalf("%s is still listed: %+v", ana, p)
		}
	}
}

func TestRemoveLocal_recordsWhoRemovedWhomAndNothingElse(t *testing.T) {
	r := removalFixture(t)
	ana := r.local(t, "ana")

	if err := r.remove(t, ana, r.admin); err != nil {
		t.Fatalf("RemoveLocal: %v", err)
	}

	recorded := r.scalar(t, `
		select count(*) from admin_events
		where action = 'person.removed' and principal_id = $1 and target = $2
		  and coalesce(detail, '{}'::jsonb) = '{}'::jsonb`, r.admin, ana)
	if recorded != 1 {
		t.Errorf("person.removed recorded %d times, want once with no detail", recorded)
	}
}

func TestRemoveLocal_refusals(t *testing.T) {
	cases := []struct {
		name string
		who  func(t *testing.T, r removal) (target, by string)
		want error
	}{
		{"a provider account comes back at its next sign-in", func(t *testing.T, r removal) (string, string) {
			return personIn(t, r.dir, "bia"), r.admin
		}, auth.ErrNotLocal},
		{"a service account has its own door", func(t *testing.T, r removal) (string, string) {
			svc, err := r.dir.CreateServiceAccount(t.Context(), "publisher", "", r.admin)
			if err != nil {
				t.Fatalf("CreateServiceAccount: %v", err)
			}
			return svc.ID, r.admin
		}, auth.ErrNotLocal},
		// Whatever its provider says: only a person signs in with a password.
		{"a machine filed as local", func(t *testing.T, r removal) (string, string) {
			if _, err := r.pool.Exec(t.Context(), `
				insert into principals (principal_id, kind, provider, subject, display)
				values ('svc_odd', 'service', 'local', 'odd', 'odd')`); err != nil {
				t.Fatalf("insert: %v", err)
			}
			return "svc_odd", r.admin
		}, auth.ErrNotLocal},
		{"yourself", func(t *testing.T, r removal) (string, string) {
			ana := r.local(t, "ana")
			return ana, ana
		}, auth.ErrRemovingYourself},
		{"nobody by that id", func(*testing.T, removal) (string, string) {
			return "usr_nobody", "usr_somebody"
		}, auth.ErrNoPrincipal},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := removalFixture(t)
			target, by := tc.who(t, r)
			if err := r.remove(t, target, by); !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

// A refusal changes nothing: the account still signs in.
func TestRemoveLocal_refused_leavesTheAccountWorking(t *testing.T) {
	r := removalFixture(t)
	ana := r.local(t, "ana")

	_ = r.remove(t, ana, ana)

	if n := r.scalar(t, `select count(*) from principals where principal_id = $1 and disabled_at is null`, ana); n != 1 {
		t.Error("a refused removal disabled the account")
	}
}

/*
The last holder of the installation's administration cannot be removed: nobody
would be left who can grant it again, and the way back is the setup token and
a shell on the server.
*/
func TestRemoveLocal_theLastInstallationAdministrator_isRefused(t *testing.T) {
	r := removalFixture(t)
	ana := r.local(t, "ana")
	installationAdmin(t, r.dir, ana)
	// The provider administrator goes first, by another route, so ana is the
	// only one left who can still get in.
	if _, err := r.pool.Exec(t.Context(),
		`update principals set disabled_at = now() where principal_id = $1`, r.admin); err != nil {
		t.Fatalf("disable: %v", err)
	}

	if err := r.remove(t, ana, "usr_operator"); !errors.Is(err, auth.ErrLastAdministrator) {
		t.Fatalf("err = %v, want ErrLastAdministrator", err)
	}
}

func TestRemoveLocal_anAdministratorWithAnotherBesideThem_isRemoved(t *testing.T) {
	r := removalFixture(t)
	ana := r.local(t, "ana")
	installationAdmin(t, r.dir, ana)

	if err := r.remove(t, ana, r.admin); err != nil {
		t.Fatalf("RemoveLocal: %v", err)
	}
}

// Administration of one company is not the installation's: removing its last
// holder leaves the installation administrator able to grant it again.
func TestRemoveLocal_theLastCompanyAdministrator_isRemoved(t *testing.T) {
	r := removalFixture(t)
	ana := r.local(t, "ana")
	if err := r.dir.SetGrants(t.Context(), ana,
		[]domain.Grant{grant("default", "", domain.RoleAdmin)}, r.admin); err != nil {
		t.Fatalf("SetGrants: %v", err)
	}

	if err := r.remove(t, ana, r.admin); err != nil {
		t.Fatalf("RemoveLocal: %v", err)
	}
}

// The account the setup token made is local too: nothing brings it back once
// it is gone, which is what the handover to a provider administrator needs.
func TestRemoveLocal_theSetupTokenAccount_isRemoved(t *testing.T) {
	r := removalFixture(t)
	if _, err := r.pool.Exec(t.Context(), `
		insert into principals (principal_id, kind, provider, subject, display)
		values ('usr_first', 'user', 'bootstrap', 'usr_first', 'Primeiro')`); err != nil {
		t.Fatalf("insert: %v", err)
	}
	installationAdmin(t, r.dir, "usr_first")

	if err := r.remove(t, "usr_first", r.admin); err != nil {
		t.Fatalf("RemoveLocal: %v", err)
	}
}

/*
Two administrators, the only two, removing each other at the same moment. Each
alone is allowed; both together leave nobody. The hold makes both read before
either writes, so a count taken before the other commits would let both
through.
*/
func TestRemoveLocal_twoAdministratorsRemovingEachOther_oneIsRefused(t *testing.T) {
	r := removalFixture(t)
	ana, bia := r.local(t, "ana"), r.local(t, "bia")
	installationAdmin(t, r.dir, ana)
	installationAdmin(t, r.dir, bia)
	if _, err := r.pool.Exec(t.Context(),
		`update principals set disabled_at = now() where principal_id = $1`, r.admin); err != nil {
		t.Fatalf("disable: %v", err)
	}
	contention := testdb.NewContention(t, 2)
	dir := auth.NewPostgres(contention.Pool)
	remove := func(target, by string) func() error {
		return func() error {
			return dir.RemoveLocal(t.Context(), auth.Removal{PrincipalID: target, By: by, At: removedAt})
		}
	}

	errs := contention.Overlap(t,
		`select 1 from principals where principal_id = any($1) for update`, []any{[]string{ana, bia}},
		[]func() error{remove(ana, bia), remove(bia, ana)})

	refused := 0
	for _, err := range errs {
		if errors.Is(err, auth.ErrLastAdministrator) {
			refused++
		}
	}
	if refused != 1 {
		t.Fatalf("errors = %v, want exactly one refused as the last administrator", errs)
	}
}
