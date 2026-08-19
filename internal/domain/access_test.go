package domain_test

import (
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
