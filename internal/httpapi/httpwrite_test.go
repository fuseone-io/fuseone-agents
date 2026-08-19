package httpapi

import (
	"slices"
	"testing"

	"github.com/fuseone/agents/internal/domain"
)

func TestMeFrom_doesNotAdvertiseInstallationOnlyPermissionsFromTheAdministrationArea(t *testing.T) {
	t.Parallel()

	me := meFrom(domain.Principal{
		ID: "usr_ana", Kind: domain.PrincipalUser,
		Grants: []domain.Grant{{Scope: adminScope, Role: domain.RoleAdmin}},
	})

	if slices.Contains(me.Can, string(domain.PermIdentityWrite)) {
		t.Error("identity:write was advertised from an ordinary area")
	}
	if slices.Contains(me.Can, string(domain.PermCompanyWrite)) {
		t.Error("company:write was advertised from an ordinary area")
	}
	if !slices.Contains(me.Can, string(domain.PermToolClassify)) {
		t.Error("ordinary administration permissions disappeared")
	}
}

func TestMeFrom_advertisesInstallationOnlyPermissionsAtTheInstallationScope(t *testing.T) {
	t.Parallel()

	me := meFrom(domain.Principal{
		ID: "usr_ana", Kind: domain.PrincipalUser,
		Grants: []domain.Grant{{Scope: identityScope, Role: domain.RoleAdmin}},
	})

	if !slices.Contains(me.Can, string(domain.PermIdentityWrite)) {
		t.Error("identity:write was not advertised at the installation scope")
	}
	if !slices.Contains(me.Can, string(domain.PermCompanyWrite)) {
		t.Error("company:write was not advertised at the installation scope")
	}
}
