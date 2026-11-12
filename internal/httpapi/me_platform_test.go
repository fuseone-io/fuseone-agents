package httpapi

import (
	"slices"
	"testing"

	"github.com/fuseone/agents/internal/domain"
)

/*
What the console is told a caller may do has to be what the server lets them do.

Some permissions are only ever checked at the platform's administration scope:
configuring integrations, classifying tools, prices and budgets, policies,
standing approvals, branding, retention, the audit export. Advertising them to
an administrator of one area put menus and buttons on the screen that answered
403 on every click.
*/

// provider:write left this list with NT-019 phase 1: connector instances are
// configured at their own scope, so an area administrator holds it for real.
// What remains the platform's (MCP servers, providers, channels) the console
// reads from platform, not from can.
var platformOnlyPerms = []domain.Permission{
	domain.PermToolClassify, domain.PermBudgetWrite,
	domain.PermPolicyWrite, domain.PermApprovalGrant, domain.PermAuditExport,
	domain.PermBrandWrite, domain.PermDataErase,
}

func areaAdministrator() domain.Principal {
	return domain.Principal{
		ID: "usr_ana", Kind: domain.PrincipalUser,
		Grants: []domain.Grant{{Scope: domain.Scope{Company: "acme", Area: "cx"}, Role: domain.RoleAdmin}},
	}
}

func TestMeFrom_anAreaAdministrator_isNotToldItCanAdministerThePlatform(t *testing.T) {
	t.Parallel()
	me := meFrom(areaAdministrator())

	for _, perm := range platformOnlyPerms {
		if slices.Contains(me.Can, string(perm)) {
			t.Errorf("%s advertised to an administrator of one area", perm)
		}
	}
	// What the area administrator really can do stays, wherever it is checked
	// in the resource's own scope.
	for _, perm := range []domain.Permission{domain.PermScopeWrite, domain.PermToolRead, domain.PermAgentPublish} {
		if !slices.Contains(me.Can, string(perm)) {
			t.Errorf("%s disappeared from an administrator of one area", perm)
		}
	}
	if len(me.Platform) != 0 {
		t.Errorf("platform = %v, want nothing held at the platform's scope", me.Platform)
	}
}

func TestMeFrom_aPlatformAdministrator_isToldWhatItHoldsThere(t *testing.T) {
	t.Parallel()
	me := meFrom(domain.Principal{
		ID: "usr_ana", Kind: domain.PrincipalUser,
		Grants: []domain.Grant{{Scope: adminScope, Role: domain.RoleAdmin}},
	})

	for _, perm := range append(platformOnlyPerms, domain.PermToolRead, domain.PermAuditRead, domain.PermProviderWrite) {
		if !slices.Contains(me.Platform, string(perm)) {
			t.Errorf("%s missing from platform", perm)
		}
	}
	for _, perm := range platformOnlyPerms {
		if !slices.Contains(me.Can, string(perm)) {
			t.Errorf("%s missing from can", perm)
		}
	}
}

// An area administrator configures its area's connector instances, so it is
// told so, and told where: the scopes a connector form may offer.
func TestMeFrom_anAreaAdministrator_isToldWhereItConfigures(t *testing.T) {
	t.Parallel()
	me := meFrom(areaAdministrator())

	if !slices.Contains(me.Can, string(domain.PermProviderWrite)) {
		t.Errorf("can = %v, want provider:write", me.Can)
	}
	if slices.Contains(me.Platform, string(domain.PermProviderWrite)) {
		t.Errorf("platform = %v, want provider:write absent: MCP servers and providers stay the platform's", me.Platform)
	}
	want := []MeScope{{Company: "acme", Area: "cx"}}
	if !slices.Equal(me.Configure, want) {
		t.Errorf("configure = %v, want %v", me.Configure, want)
	}
}

// An author configures nothing.
func TestMeFrom_anAuthor_configuresNowhere(t *testing.T) {
	t.Parallel()
	me := meFrom(domain.Principal{
		ID: "usr_ana", Kind: domain.PrincipalUser,
		Grants: []domain.Grant{{Scope: domain.Scope{Company: "acme", Area: "cx"}, Role: domain.RoleAuthor}},
	})
	if len(me.Configure) != 0 || slices.Contains(me.Can, string(domain.PermProviderWrite)) {
		t.Errorf("configure = %v, can = %v, want neither to configure", me.Configure, me.Can)
	}
}
