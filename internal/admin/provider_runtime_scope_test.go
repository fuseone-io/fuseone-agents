package admin

import (
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/settings"
)

/*
A provider row is read only at a scope this binary knows (NT-019, phase 3
readers): the installation, a company, or an area, each with exactly the
columns it means. A row of another kind, such as one a later release adds, or
whose columns contradict its kind, is no scope here: reading it as some scope
would hand its key to runs it was never meant for.
*/
func TestProviderRuntimeScope_knowsOnlyTheThreeScopesItMeans(t *testing.T) {
	t.Parallel()
	cases := []struct {
		kind          settings.ScopeKind
		company, area string
		want          domain.Scope
		known         bool
	}{
		{settings.ScopeInstallation, "", "", domain.Scope{Company: domain.Installation}, true},
		{settings.ScopeCompany, "acme", "", domain.Scope{Company: "acme"}, true},
		{settings.ScopeArea, "acme", "cx", domain.Scope{Company: "acme", Area: "cx"}, true},
		{settings.ScopeInstallation, "acme", "", domain.Scope{}, false},
		{settings.ScopeCompany, "acme", "cx", domain.Scope{}, false},
		{settings.ScopeCompany, "", "", domain.Scope{}, false},
		{settings.ScopeArea, "acme", "", domain.Scope{}, false},
		{settings.ScopeArea, "", "cx", domain.Scope{}, false},
		{"team", "acme", "cx", domain.Scope{}, false},
	}
	for _, c := range cases {
		got, known := providerRuntimeScope(c.kind, domain.Scope{Company: domain.CompanyID(c.company), Area: domain.AreaID(c.area)})
		if known != c.known || got != c.want {
			t.Errorf("providerRuntimeScope(%s, %q, %q) = %v, %v; want %v, %v", c.kind, c.company, c.area, got, known, c.want, c.known)
		}
	}
}
