package domain_test

import (
	"slices"
	"testing"

	"github.com/fuseone/agents/internal/domain"
)

/*
A principal acting through a connection an area owns acts with the grants it
holds inside that area, and no others (NT-019, phase 4).

Whoever owns a Slack app holds its signing secret, so they can sign any click
as any account bound to it. The person behind the account may hold far more
than the area: a grant over the company, another area's, the platform's. Kept
whole, a forged click would decide with all of it.
*/
func TestPrincipalWithin_keepsOnlyTheGrantsInsideTheScope(t *testing.T) {
	t.Parallel()
	cx := domain.Scope{Company: "acme", Area: "cx"}
	inside := domain.Grant{Scope: cx, Role: domain.RoleApprover}
	person := domain.Principal{
		ID: "usr_ana", Kind: domain.PrincipalUser,
		Grants: []domain.Grant{
			{Scope: domain.Scope{Company: domain.Installation}, Role: domain.RoleAdmin},
			{Scope: domain.Scope{Company: "acme"}, Role: domain.RoleApprover},
			{Scope: domain.Scope{Company: "acme", Area: "finance"}, Role: domain.RoleApprover},
			inside,
		},
	}

	got := person.Within(cx)

	if !slices.Equal(got.Grants, []domain.Grant{inside}) {
		t.Fatalf("grants = %+v, want only the one inside acme/cx", got.Grants)
	}
	if got.ID != person.ID || got.Kind != person.Kind {
		t.Errorf("principal = %+v, want the same person with fewer grants", got)
	}
	if got.Can(domain.PermApprovalAct, domain.Scope{Company: "acme", Area: "finance"}) {
		t.Error("the narrowed principal still decides for acme/finance")
	}
	if !got.Can(domain.PermApprovalAct, cx) {
		t.Error("the narrowed principal no longer decides for acme/cx")
	}
}

// The installation contains every scope, so a connection of the platform's
// narrows nothing: every binding there keeps acting as it does today.
func TestPrincipalWithin_theInstallation_keepsEveryGrant(t *testing.T) {
	t.Parallel()
	person := domain.Principal{
		ID: "usr_ana", Kind: domain.PrincipalUser,
		Grants: []domain.Grant{
			{Scope: domain.Scope{Company: domain.Installation}, Role: domain.RoleAdmin},
			{Scope: domain.Scope{Company: "acme", Area: "cx"}, Role: domain.RoleApprover},
		},
	}

	got := person.Within(domain.Scope{Company: domain.Installation})

	if !slices.Equal(got.Grants, person.Grants) {
		t.Fatalf("grants = %+v, want all of them", got.Grants)
	}
}
