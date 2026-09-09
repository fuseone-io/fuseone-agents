package auth_test

import (
	"context"
	"slices"
	"testing"

	"github.com/fuseone/agents/internal/domain"
)

/*
Who this installation knows may decide an approval here.

The platform could always answer "may this person decide" and never "which
people may". Asking somebody to decide requires the second, and the two have to
give the same answer or a notification goes to somebody the button will refuse
— or, worse, is withheld from somebody it would have accepted.

The rule is domain.Scope.Contains, and this is its second statement, in SQL. So
the table below drives both and demands they agree, case by case. The direction
that fails quietly is the dangerous one: a grant on one area must never reach
the company it belongs to, or an approver for a single team is notified about —
and can decide — everything the company runs.
*/
func TestApproversIn_agreesWithScopeContains(t *testing.T) {
	dir, pool := directoryFor(t)

	for _, held := range []domain.Scope{
		{Company: domain.Installation},
		{Company: "acme"},
		{Company: "acme", Area: "ops"},
		{Company: "acme", Area: "cx"},
		{Company: "other", Area: "ops"},
	} {
		person := personIn(t, dir, "approver-"+string(held.Company)+"-"+string(held.Area))
		if err := dir.SetGrants(t.Context(), person,
			[]domain.Grant{{Scope: held, Role: domain.RoleApprover}}, "test"); err != nil {
			t.Fatalf("SetGrants %+v: %v", held, err)
		}
		t.Cleanup(func() { _ = person })

		for _, run := range []domain.Scope{
			{Company: "acme", Area: "ops"},
			{Company: "acme", Area: "cx"},
			{Company: "acme"},
			{Company: "other", Area: "ops"},
		} {
			listed := listsApprover(t, dir, run, person)
			if want := held.Contains(run); listed != want {
				t.Errorf("grant %+v over run %+v: listed = %v, Contains = %v",
					held, run, listed, want)
			}
		}
	}
	_ = pool
}

// An admin may decide and is deliberately not on this list. One held at the
// installation covers every company, so a notification built from the
// permission would tell every administrator about every parked run there is.
func TestApproversIn_anAdministrator_isNotNotified(t *testing.T) {
	dir, _ := directoryFor(t)
	ops := domain.Scope{Company: "acme", Area: "ops"}

	boss := personIn(t, dir, "boss")
	if err := dir.SetGrants(t.Context(), boss,
		[]domain.Grant{{Scope: domain.Scope{Company: domain.Installation}, Role: domain.RoleAdmin}},
		"test"); err != nil {
		t.Fatalf("SetGrants: %v", err)
	}

	if listsApprover(t, dir, ops, boss) {
		t.Error("an administrator was put on the notification list")
	}
}

/*
Somebody switched off is not asked to decide.

The decision path already refuses them, so notifying them produces a private
message whose button answers that their account is gone. The listing that feeds
the console does not filter disabled people; this one has to.
*/
func TestApproversIn_aDisabledPerson_isNotNotified(t *testing.T) {
	dir, pool := directoryFor(t)
	ops := domain.Scope{Company: "acme", Area: "ops"}

	gone := personIn(t, dir, "gone")
	if err := dir.SetGrants(t.Context(), gone,
		[]domain.Grant{{Scope: ops, Role: domain.RoleApprover}}, "test"); err != nil {
		t.Fatalf("SetGrants: %v", err)
	}
	if !listsApprover(t, dir, ops, gone) {
		t.Fatal("the fixture is wrong: they should be listed before being disabled")
	}

	if _, err := pool.Exec(t.Context(),
		`update principals set disabled_at = now() where principal_id = $1`, gone); err != nil {
		t.Fatalf("disable: %v", err)
	}
	if listsApprover(t, dir, ops, gone) {
		t.Error("a disabled account was put on the notification list")
	}
}

/*
Only people are told.

A service account holding an API token, or an agent principal delegated
authority for a run, may carry the role and legitimately decide through the
API. Neither has anybody to send a private message to, and putting them on the
list produces a message addressed to a machine or an address that does not
exist.

The rest of this suite creates people and only people, so the filter that keeps
them out had nothing testing it — which is how a clause survives long enough to
be deleted by somebody tidying up.
*/
func TestApproversIn_aServiceOrAgentPrincipal_isNotNotified(t *testing.T) {
	dir, pool := directoryFor(t)
	ops := domain.Scope{Company: "acme", Area: "ops"}

	// A person with the same grant, so a green test cannot be the fixture
	// failing to grant anything.
	person := personIn(t, dir, "ana")
	machines := map[string]string{"service": "svc_tokenholder", "agent": "agt_delegated"}
	for kind, id := range machines {
		if _, err := pool.Exec(t.Context(),
			`insert into principals (principal_id, kind, display) values ($1, $2, $3)`,
			id, kind, kind); err != nil {
			t.Fatalf("seed a %s principal: %v", kind, err)
		}
	}
	for _, id := range append([]string{person}, "svc_tokenholder", "agt_delegated") {
		if err := dir.SetGrants(t.Context(), id,
			[]domain.Grant{{Scope: ops, Role: domain.RoleApprover}}, "test"); err != nil {
			t.Fatalf("SetGrants %s: %v", id, err)
		}
	}

	who, err := dir.ApproversIn(t.Context(), ops)
	if err != nil {
		t.Fatalf("ApproversIn: %v", err)
	}
	if !slices.Contains(who, domain.UserID(person)) {
		t.Fatalf("who = %v, want the person listed", who)
	}
	for kind, id := range machines {
		if slices.Contains(who, domain.UserID(id)) {
			t.Errorf("a %s principal was put on the notification list", kind)
		}
	}
}

// Two grants reaching the same run name one person once. The list addresses
// messages, and a duplicate is a second private message about one question.
func TestApproversIn_grantedTwiceOverTheSameRun_isNamedOnce(t *testing.T) {
	dir, _ := directoryFor(t)
	ops := domain.Scope{Company: "acme", Area: "ops"}

	twice := personIn(t, dir, "twice")
	if err := dir.SetGrants(t.Context(), twice, []domain.Grant{
		{Scope: domain.Scope{Company: "acme"}, Role: domain.RoleApprover},
		{Scope: ops, Role: domain.RoleApprover},
	}, "test"); err != nil {
		t.Fatalf("SetGrants: %v", err)
	}

	who, err := dir.ApproversIn(t.Context(), ops)
	if err != nil {
		t.Fatalf("ApproversIn: %v", err)
	}
	if n := slices.Index(who, domain.UserID(twice)); n < 0 {
		t.Fatalf("who = %v, want it to name them", who)
	}
	named := 0
	for _, id := range who {
		if id == domain.UserID(twice) {
			named++
		}
	}
	if named != 1 {
		t.Errorf("named %d times, want once", named)
	}
}

type approverLister interface {
	ApproversIn(ctx context.Context, scope domain.Scope) ([]domain.UserID, error)
}

func listsApprover(t *testing.T, dir approverLister, run domain.Scope, person string) bool {
	t.Helper()
	who, err := dir.ApproversIn(t.Context(), run)
	if err != nil {
		t.Fatalf("ApproversIn %+v: %v", run, err)
	}
	return slices.Contains(who, domain.UserID(person))
}
