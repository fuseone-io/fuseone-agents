package main

import (
	"context"
	"fmt"
)

// expectedServers answers the engine's question about a missing tool's
// server: configured and enabled means its absence from this worker's
// catalogue is an outage to back off from, anything else means it was a
// decision (#148). Consulted only when a pack tool is already undescribable,
// so the settings read is rare.
type expectedServers struct{ servers Servers }

func (e expectedServers) Expected(ctx context.Context, name string) (bool, error) {
	configured, err := e.servers.MCPServers(ctx)
	if err != nil {
		return false, fmt.Errorf("read configured tool servers: %w", err)
	}
	for _, server := range configured {
		if server.Name == name {
			return server.Enabled, nil
		}
	}
	return false, nil
}
