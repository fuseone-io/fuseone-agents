package domain_test

import (
	"slices"
	"testing"

	"github.com/fuseone/agents/internal/domain"
)

func TestContains_companyWideGrantReachesItsAreas(t *testing.T) {
	t.Parallel()

	// PRD §3.1: the hierarchy inherits downwards. A grant with no area is the
	// whole company, which is how the first administrator of an installation
	// governs areas that did not exist when they were granted.
	company := domain.Scope{Company: "acme"}
	if !company.Contains(domain.Scope{Company: "acme", Area: "cx"}) {
		t.Error("a company-wide grant did not reach one of its areas")
	}
}

func TestContains_neverWidensSidewaysOrUpwards(t *testing.T) {
	t.Parallel()

	cx := domain.Scope{Company: "acme", Area: "cx"}

	if cx.Contains(domain.Scope{Company: "acme", Area: "marketing"}) {
		t.Error("an area grant reached a sibling area")
	}
	if cx.Contains(domain.Scope{Company: "acme"}) {
		t.Error("an area grant reached the company above it")
	}
	if (domain.Scope{Company: "acme"}).Contains(domain.Scope{Company: "other", Area: "cx"}) {
		t.Error("a grant reached another company")
	}
}

func TestCan_companyCuratorGovernsANewArea(t *testing.T) {
	t.Parallel()

	curator := domain.Principal{
		Grants: []domain.Grant{{Scope: domain.Scope{Company: "acme"}, Role: domain.RoleCurator}},
	}
	// The area is created after the grant, which is the ordinary case: areas
	// appear as the installation is used.
	if !curator.Can(domain.PermToolClassify, domain.Scope{Company: "acme", Area: "financeiro"}) {
		t.Error("a company curator could not govern an area created later")
	}
}

func TestCan_installationAdminReachesEveryCompany(t *testing.T) {
	t.Parallel()

	admin := domain.Principal{
		Grants: []domain.Grant{{Scope: domain.Scope{Company: domain.Installation}, Role: domain.RoleAdmin}},
	}
	if !admin.Can(domain.PermCompanyWrite, domain.Scope{Company: domain.Installation}) {
		t.Error("an installation admin could not govern companies")
	}
	if !admin.Can(domain.PermToolClassify, domain.Scope{Company: "newco", Area: "devops"}) {
		t.Error("an installation admin did not reach a company area")
	}
}

/*
The approver role still carries the permission it is named for.

A notification list is built from the role rather than from the permission,
because the permission also belongs to admin and an installation-wide admin
would be told about every parked run in every company — that is not a
notification, it is noise.

Naming the role is only sound while the role means what it says. If `approver`
ever stops carrying `approval:act`, the list becomes a list of people who will
be handed a 403 in a private message, and this fails on the commit that breaks
it rather than in somebody's Slack.
*/
func TestRoleApprover_stillCarriesApprovalAct(t *testing.T) {
	t.Parallel()

	if !domain.RoleApprover.Allows(domain.PermApprovalAct) {
		t.Fatal("approver no longer carries approval:act; the notification list is now wrong")
	}
}

/*
Which roles allow an act is read from the table, never listed again by hand.

Naming somebody to be told about an approval has to offer the people whose
button will accept them, and that is a property of the grants table — a role
name is a proxy for it that was wrong the day admin gained the act. Written out
a second time, the copy is what drifts: the table gains a role, the copy does
not, and the symptom is somebody offered a decision the platform then refuses.
*/
func TestRolesAllowing_isEveryRoleTheTableGivesTheAct(t *testing.T) {
	t.Parallel()

	for _, perm := range []domain.Permission{
		domain.PermApprovalAct, domain.PermAgentPublish, domain.PermDataErase,
	} {
		allowed := domain.RolesAllowing(perm)
		for _, role := range domain.Roles() {
			listed := slices.Contains(allowed, role)
			if want := role.Allows(perm); listed != want {
				t.Errorf("%s in RolesAllowing(%s) = %v, Allows = %v",
					role, perm, listed, want)
			}
		}
	}
}

// And the one this platform asks about is both of them, which is the fact the
// approval fan-out and the console both stand on.
func TestRolesAllowing_theApprovalAct_isTheApproverAndTheAdministrator(t *testing.T) {
	t.Parallel()

	if got := domain.RolesAllowing(domain.PermApprovalAct); !slices.Equal(
		got, []domain.Role{domain.RoleAdmin, domain.RoleApprover},
	) {
		t.Errorf("RolesAllowing(approval:act) = %v", got)
	}
}
