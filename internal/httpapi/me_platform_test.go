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

var platformOnlyPerms = []domain.Permission{
	domain.PermProviderWrite, domain.PermToolClassify, domain.PermBudgetWrite,
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

	for _, perm := range append(platformOnlyPerms, domain.PermToolRead, domain.PermAuditRead) {
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
