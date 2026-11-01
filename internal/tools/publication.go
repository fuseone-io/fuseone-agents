package tools

import (
	"maps"
	"slices"

	"github.com/fuseone/agents/internal/domain"
)

// Publication is the catalogue as the administration records it: every tool,
// and the servers whose discovery this catalogue holds in full.
//
// A server is in it once AddServer succeeded, which is after its tools were
// listed and before anything else could see them, and until RemoveServer.
// Read under one lock with the tools, so no server is named whose tools are
// not all there. Which configuration each discovery was made under is the
// connecting side's knowledge, not the catalogue's; it fills that in.
func (c *Catalog) Publication() domain.ToolPublication {
	c.mu.RLock()
	defer c.mu.RUnlock()
	names := slices.Sorted(maps.Keys(c.sessions))
	servers := make([]domain.PublishedServer, len(names))
	for i, name := range names {
		servers[i] = domain.PublishedServer{Name: name}
	}
	return domain.ToolPublication{Servers: servers, Entries: c.entriesLocked()}
}
