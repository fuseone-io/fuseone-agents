package auth_test

import (
	"context"
	"slices"
	"testing"

	"github.com/fuseone/agents/internal/auth"
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

/*
An administrator is not broadcast to, and may be named.

Two different questions that a single role name answered wrongly for one of
them. An admin holds the approval act, so their button works — but one held at
the installation covers every company, and announcing by the act alone would
tell every administrator about every parked run there is. That is why the
broadcast asks for the Approver role and not for the permission.

Naming is the other direction: an agent's owner picked this person, one message
to one person who may genuinely decide. Offering them was the missing half —
the screen listed the role, so the one colleague an owner most wants to name
was not in the list, and would have been dropped in silence if they had been.
*/
func TestApproversIn_anAdministrator_isNotBroadcastToButMayBeNamed(t *testing.T) {
	dir, _ := directoryFor(t)
	ops := domain.Scope{Company: "acme", Area: "ops"}

	boss := personIn(t, dir, "boss")
	if err := dir.SetGrants(t.Context(), boss,
		[]domain.Grant{{Scope: domain.Scope{Company: domain.Installation}, Role: domain.RoleAdmin}},
		"test"); err != nil {
		t.Fatalf("SetGrants: %v", err)
	}

	// listsApprover already demands the screen and the named path agree about
	// this person, so the one assertion here is the broadcast staying narrow.
	if listsApprover(t, dir, ops, boss) {
		t.Error("an administrator was announced to without being named")
	}
	deciding, err := dir.DecidersIn(t.Context(), ops)
	if err != nil {
		t.Fatalf("DecidersIn: %v", err)
	}
	if !slices.Contains(deciding, domain.UserID(boss)) {
		t.Error("an administrator cannot be named, and their button would accept them")
	}
}

/*
And a role that cannot decide is neither broadcast to nor nameable.

The curator is the case worth writing down: they configure almost everything
this platform has — packs, policies, budgets, providers — and hold no approval
act at all. A list built from "powerful roles" rather than from the act would
have them in it, and every name in it is a private message whose button refuses
the person who receives it.
*/
func TestDecidersIn_aRoleWithoutTheAct_isNeitherToldNorOffered(t *testing.T) {
	dir, _ := directoryFor(t)
	ops := domain.Scope{Company: "acme", Area: "ops"}

	for _, role := range []domain.Role{domain.RoleCurator, domain.RoleAuthor, domain.RoleAuditor} {
		person := personIn(t, dir, "holder-"+string(role))
		if err := dir.SetGrants(t.Context(), person,
			[]domain.Grant{{Scope: domain.Scope{Company: "acme"}, Role: role}},
			"test"); err != nil {
			t.Fatalf("SetGrants %s: %v", role, err)
		}

		if listsApprover(t, dir, ops, person) {
			t.Errorf("a %s was announced to", role)
		}
		deciding, err := dir.DecidersIn(t.Context(), ops)
		if err != nil {
			t.Fatalf("DecidersIn: %v", err)
		}
		if slices.Contains(deciding, domain.UserID(person)) {
			t.Errorf("a %s can be named, and the button would refuse them", role)
		}
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

/*
approverLister is the three readings of one question, named together because
what they must not do is disagree by accident.

ApproversIn is who a parked run is announced to when nobody was named — the
broadcast, deliberately narrower than the act, because an administrator held at
the installation would otherwise be told about every parked run there is.
DecidersIn is who may decide, which is what an explicitly named person is
checked against. DecidersNamed is that same set with the names attached, and it
is what a screen offers.
*/
type approverLister interface {
	ApproversIn(ctx context.Context, scope domain.Scope) ([]domain.UserID, error)
	DecidersIn(ctx context.Context, scope domain.Scope) ([]domain.UserID, error)
	DecidersNamed(ctx context.Context, scope domain.Scope) ([]auth.Eligible, error)
}

/*
listsApprover asks all three readings and demands the two that must agree do.

The screen offers a list; the named path decides whether the person it reached
may act. Written apart those two drift, and the drift is invisible — a screen
offering somebody the fan-out will silently drop, or hiding somebody it would
message. So they are one query, and this demands it person by person.

The broadcast is allowed to be narrower and never wider: somebody announced to
without being named must be somebody who may decide, or the platform is sending
approval requests to people whose button refuses them.
*/
func listsApprover(t *testing.T, dir approverLister, run domain.Scope, person string) bool {
	t.Helper()
	who, err := dir.ApproversIn(t.Context(), run)
	if err != nil {
		t.Fatalf("ApproversIn %+v: %v", run, err)
	}
	listed := slices.Contains(who, domain.UserID(person))

	deciding, err := dir.DecidersIn(t.Context(), run)
	if err != nil {
		t.Fatalf("DecidersIn %+v: %v", run, err)
	}
	mayDecide := slices.Contains(deciding, domain.UserID(person))
	if listed && !mayDecide {
		t.Errorf("run %+v: %s is announced to and may not decide", run, person)
	}

	named, err := dir.DecidersNamed(t.Context(), run)
	if err != nil {
		t.Fatalf("DecidersNamed %+v: %v", run, err)
	}
	offered := slices.ContainsFunc(named, func(one auth.Eligible) bool {
		return one.ID == domain.UserID(person)
	})
	if offered != mayDecide {
		t.Errorf("run %+v: the screen offers %s = %v, the named path accepts it = %v",
			run, person, offered, mayDecide)
	}
	// And a name a person recognises, because a screen showing identifiers is
	// a screen where somebody picks the wrong colleague.
	for _, one := range named {
		if one.Display == "" {
			t.Errorf("%s is offered with no name", one.ID)
		}
	}
	return listed
}
