package channel_test

import (
	"testing"

	"github.com/fuseone/agents/internal/channel"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/settings"
)

/*
A connection is read only at a scope this binary knows (NT-019, phase 4
readers): the installation, a company, or an area, each with exactly the
columns it means. Anything else is no scope here. Read as some scope, the row's
token would speak, and its signing secret would admit, for runs it was never
meant for.
*/
func TestConnectionScope_knowsOnlyTheThreeScopesItMeans(t *testing.T) {
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
		// The installation's sentinel contains every scope: written as a
		// company or an area's company it would reach every run.
		{settings.ScopeCompany, "*", "", domain.Scope{}, false},
		{settings.ScopeArea, "*", "cx", domain.Scope{}, false},
	}
	for _, c := range cases {
		stored := domain.Scope{Company: domain.CompanyID(c.company), Area: domain.AreaID(c.area)}
		got, known := channel.ConnectionScope(c.kind, stored)
		if known != c.known || got != c.want {
			t.Errorf("ConnectionScope(%s, %q, %q) = %v, %v; want %v, %v",
				c.kind, c.company, c.area, got, known, c.want, c.known)
		}
	}
}

// installed is an arrival on a connection of the installation's, which serves
// every scope: what every connection was before areas had their own.
func installed(name string) channel.Via {
	return channel.Via{Name: name, Scope: domain.Scope{Company: domain.Installation}}
}
