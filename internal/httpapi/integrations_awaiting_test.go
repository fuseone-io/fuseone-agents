package httpapi

import (
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

// The console says a server waits for a person only while that is what the
// worker saw of the server as configured now (#483, delivery 3b): an
// observation under an older configuration, or one a binary that predates the
// wait wrote over without clearing it, says nothing about it any more.
func TestListIntegrations_aServerWaitingForAPerson_saysSoOnlyUnderItsCurrentConfiguration(t *testing.T) {
	t.Parallel()
	generation := time.Date(2026, 11, 27, 9, 0, 0, 0, time.UTC)
	waiting := domain.IntegrationHealth{
		Name: "dpg", Reachable: false, ObservedAt: generation.Add(time.Minute),
		AwaitingPerson: true, AwaitingGeneration: generation,
	}
	reachable, older := waiting, waiting
	reachable.Reachable = true
	older.AwaitingGeneration = generation.Add(-time.Second)

	cases := map[string]struct {
		seen domain.IntegrationHealth
		want bool
	}{
		"waiting under the current configuration": {waiting, true},
		"observed under an older configuration":   {older, false},
		"reachable, written by an older binary":   {reachable, false},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			admin := &fakeAdmin{servers: []domain.MCPServer{{
				Name: "dpg", Transport: domain.TransportHTTP, URL: "https://mcp.example/dpg",
				Enabled: true, UpdatedAt: generation,
			}}}

			resp, err := serverWith(t, admin).WithHealth(fakeHealth{"dpg": c.seen}).
				ListIntegrations(as(domain.RoleCurator), openapi.ListIntegrationsRequestObject{})
			if err != nil {
				t.Fatalf("ListIntegrations: %v", err)
			}
			got := resp.(openapi.ListIntegrations200JSONResponse).McpServers[0].Health
			if got == nil || (got.AwaitingPerson != nil && *got.AwaitingPerson) != c.want {
				t.Errorf("health = %+v, want awaitingPerson %v", got, c.want)
			}
		})
	}
}

// A server passed by flag has no configuration to match: it never waits.
func TestListIntegrations_anObservedServerNobodyConfigured_neverWaitsForAPerson(t *testing.T) {
	t.Parallel()
	resp, err := serverWith(t, &fakeAdmin{}).WithHealth(fakeHealth{"dpg": {
		Name: "dpg", ObservedAt: time.Now(), AwaitingPerson: true,
	}}).ListIntegrations(as(domain.RoleCurator), openapi.ListIntegrationsRequestObject{})
	if err != nil {
		t.Fatalf("ListIntegrations: %v", err)
	}
	if got := resp.(openapi.ListIntegrations200JSONResponse).McpServers[0].Health; got == nil ||
		(got.AwaitingPerson != nil && *got.AwaitingPerson) {
		t.Errorf("health = %+v, want no wait", got)
	}
}
