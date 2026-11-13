package tools

import "github.com/fuseone/agents/internal/domain"

/*
Where a catalogued tool belongs (NT-019).

A tool is its server's: an area's server reaches that area's runs, and the Gate
reads the scope from here. The administration stores each server at its own
scope, and the worker hands it over when it connects the server (WithScope).
A server added with none, as a --mcp flag adds one, is the installation's.
A tool this catalogue does not know is nobody's.
*/
func (c *Catalog) Scope(id domain.ToolID) (domain.Scope, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.entries[id]
	if !ok || !entry.OnSurface {
		return domain.Scope{}, false
	}
	return c.serverScopeLocked(entry.Server), true
}

func (c *Catalog) serverScopeLocked(server string) domain.Scope {
	if scope, ok := c.scopes[server]; ok {
		return scope
	}
	return installationScope
}

var installationScope = domain.Scope{Company: domain.Installation}

// serverScope is a server's scope as AddServer records it.
func serverScope(given domain.Scope) domain.Scope {
	if given == (domain.Scope{}) {
		return installationScope
	}
	return given
}

// WithScope is where the server belongs, from the row that configures it.
func WithScope(scope domain.Scope) ServerOption {
	return func(options *serverOptions) {
		options.scope = scope
	}
}

// rulingsInForce is domain.RulingsInForce over this catalogue's servers.
func (c *Catalog) rulingsInForce(given []domain.ToolClassification) []domain.ToolClassification {
	return domain.RulingsInForce(given, func(id domain.ToolID) (domain.Scope, bool) {
		entry, ok := c.entries[id]
		if !ok {
			return domain.Scope{}, false
		}
		return c.serverScopeLocked(entry.Server), true
	})
}
