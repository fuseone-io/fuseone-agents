package tools

import "github.com/fuseone/agents/internal/domain"

/*
Scope answers where a catalogued tool belongs (NT-019).

Every MCP server is the installation's: the administration stores each one at
the installation scope, and nothing writes one anywhere else until the readers
of NT-019 phase 2 learn a server's own scope. So a tool this catalogue knows,
on the surface, is the installation's, and one it does not know is nobody's.
When servers gain scopes, this is the line that reads the server's.
*/
func (c *Catalog) Scope(id domain.ToolID) (domain.Scope, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.entries[id]
	if !ok || !entry.OnSurface {
		return domain.Scope{}, false
	}
	return domain.Scope{Company: domain.Installation}, true
}
