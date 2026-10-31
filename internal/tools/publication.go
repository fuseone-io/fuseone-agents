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
// not all there, and no tool arrives without its server being judged.
func (c *Catalog) Publication() domain.ToolPublication {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return domain.ToolPublication{
		Servers: slices.Sorted(maps.Keys(c.sessions)),
		Entries: c.entriesLocked(),
	}
}
