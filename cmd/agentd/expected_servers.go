package main

import (
	"context"
	"fmt"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/domain"
)

// expectedServers answers the engine's question about a missing tool's
// server: configured and enabled means its absence from this worker's
// catalogue is an outage to back off from, anything else means it was a
// decision (#148). Consulted only when a pack tool is already undescribable,
// so the settings read is rare.
//
// health and discoverers answer whether an expected server waits for a person;
// either missing, it never does.
type expectedServers struct {
	servers     Servers
	health      observations
	discoverers personalDiscoverers
}

// observations is what the worker last recorded about one server.
type observations interface {
	Of(ctx context.Context, name string) (domain.IntegrationHealth, bool, error)
}

var _ observations = (*admin.Health)(nil)

func (e expectedServers) Expected(ctx context.Context, name string) (bool, error) {
	server, found, err := e.configured(ctx, name)
	return found && server.Enabled, err
}

/*
AwaitingPerson is whether a run is refused for good on the server (#483,
delivery 3b): enabled, observed waiting for a person and unreachable under the
configuration as it is now, and nobody connected when the store is asked
again. The observation alone is not enough: a person who connected since, or
the configuration changing, ends the wait before any worker records it, and a
binary that predates the wait writes over the row without clearing it.
*/
func (e expectedServers) AwaitingPerson(ctx context.Context, name string) (bool, error) {
	if e.health == nil || e.discoverers == nil {
		return false, nil
	}
	server, found, err := e.configured(ctx, name)
	if err != nil || !found || !server.Enabled {
		return false, err
	}
	obs, observed, err := e.health.Of(ctx, name)
	if err != nil {
		return false, fmt.Errorf("read what was observed of %s: %w", name, err)
	}
	if !observed || !obs.AwaitingPerson || obs.Reachable || !obs.AwaitingGeneration.Equal(server.UpdatedAt) {
		return false, nil
	}
	_, connected, err := e.discoverers.MCPPersonalDiscoverer(ctx, server.Scope, name)
	if err != nil {
		return false, fmt.Errorf("read who can discover %s: %w", name, err)
	}
	return !connected, nil
}

func (e expectedServers) configured(ctx context.Context, name string) (domain.MCPServer, bool, error) {
	configured, err := e.servers.MCPServers(ctx)
	if err != nil {
		return domain.MCPServer{}, false, fmt.Errorf("read configured tool servers: %w", err)
	}
	for _, server := range configured {
		if server.Name == name {
			return server, true, nil
		}
	}
	return domain.MCPServer{}, false, nil
}
