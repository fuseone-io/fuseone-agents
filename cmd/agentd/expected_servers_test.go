package main

import (
	"context"
	"testing"

	"github.com/fuseone/agents/internal/domain"
)

type configuredServers []domain.MCPServer

func (c configuredServers) MCPServers(context.Context) ([]domain.MCPServer, error) {
	return c, nil
}

func (configuredServers) MCPCredentials(context.Context, string) (domain.MCPCredentials, error) {
	return domain.MCPCredentials{}, nil
}

// The engine parks a turn over a missing server only when somebody still
// expects that server to be there: configured and enabled. Disabled or
// deleted means the absence was a decision (#148).
func TestExpected_onlyAConfiguredEnabledServer_isExpected(t *testing.T) {
	t.Parallel()

	e := expectedServers{servers: configuredServers{
		{Name: "grafana", Enabled: true},
		{Name: "jira", Enabled: false},
	}}

	cases := map[string]bool{"grafana": true, "jira": false, "removed": false}
	for name, want := range cases {
		got, err := e.Expected(context.Background(), name)
		if err != nil {
			t.Fatalf("Expected(%s): %v", name, err)
		}
		if got != want {
			t.Errorf("Expected(%s) = %v, want %v", name, got, want)
		}
	}
}
