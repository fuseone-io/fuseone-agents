package admin

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/settings"
)

/*
Where an MCP server is stored (NT-019, phase 2 readers).

A server's name is unique across the installation (migration 0108), so the
name finds the one row, and every reader opens it at its own scope: the
credential is sealed under that scope, and a rewrite of the row, such as an
OAuth refresh, has to land on it and not create an installation row beside it.

Enabled or not: a reader that reports a server as disabled has to find it.
A name no row holds answers the installation's key, where the caller's own
not-found handling stays what it was.
*/
func (i *Integrations) mcpServerAt(ctx context.Context, name string) (settings.ScopeKind, domain.Scope, error) {
	var kind, company, area string
	err := i.pool.QueryRow(ctx, `
		select scope_kind, company_id, area_id from settings
		where kind = $1 and name = $2`, string(settings.KindMCPServer), name,
	).Scan(&kind, &company, &area)
	if errors.Is(err, pgx.ErrNoRows) {
		return settings.ScopeInstallation, domain.Scope{}, nil
	}
	if err != nil {
		return "", domain.Scope{}, fmt.Errorf("admin: find MCP server %s: %w", name, err)
	}
	return settings.ScopeKind(kind), domain.Scope{Company: domain.CompanyID(company), Area: domain.AreaID(area)}, nil
}

// mcpServerRuntimeScope is the scope a run must be inside to reach the
// server's tools: the installation's contains every run.
func mcpServerRuntimeScope(kind settings.ScopeKind, stored domain.Scope) domain.Scope {
	if kind == settings.ScopeInstallation {
		return domain.Scope{Company: domain.Installation}
	}
	return stored
}

// storedRuntimeScope is mcpServerRuntimeScope over a row's raw columns, for a
// reader that scans them itself, such as the rulings.
func storedRuntimeScope(kind, company, area string) domain.Scope {
	return mcpServerRuntimeScope(settings.ScopeKind(kind),
		domain.Scope{Company: domain.CompanyID(company), Area: domain.AreaID(area)})
}

// serverScopes is the runtime scope of every configured server, by name. A
// server no row holds, one a --mcp flag connected, is absent and is the
// installation's to whoever reads this.
func serverScopes(ctx context.Context, q settings.DB) (map[string]domain.Scope, error) {
	rows, err := q.Query(ctx, `
		select name, scope_kind, company_id, area_id from settings where kind = $1`,
		string(settings.KindMCPServer))
	if err != nil {
		return nil, fmt.Errorf("admin: read where the servers are: %w", err)
	}
	defer rows.Close()
	out := map[string]domain.Scope{}
	for rows.Next() {
		var name, kind, company, area string
		if err := rows.Scan(&name, &kind, &company, &area); err != nil {
			return nil, fmt.Errorf("admin: read where the servers are: %w", err)
		}
		out[name] = storedRuntimeScope(kind, company, area)
	}
	return out, rows.Err()
}

// rulingsInForce is the ruling each listed tool is under, chosen as the worker
// chooses it (domain.RulingsInForce): the platform's over the owner's, and the
// owner only at the scope of the tool's server.
func (c *Curator) rulingsInForce(
	ctx context.Context, entries []domain.ToolEntry, rulings []domain.ToolClassification,
) (map[domain.ToolID]domain.ToolClassification, error) {
	scopes, err := serverScopes(ctx, c.pool)
	if err != nil {
		return nil, err
	}
	serverOf := make(map[domain.ToolID]string, len(entries))
	for _, e := range entries {
		serverOf[e.ID] = e.Server
	}
	installation := domain.Scope{Company: domain.Installation}
	inForce := domain.RulingsInForce(rulings, func(id domain.ToolID) (domain.Scope, bool) {
		server, ok := serverOf[id]
		if !ok {
			return domain.Scope{}, false
		}
		if scope, configured := scopes[server]; configured {
			return scope, true
		}
		return installation, true
	})
	out := make(map[domain.ToolID]domain.ToolClassification, len(inForce))
	for _, r := range inForce {
		out[r.Tool] = r
	}
	return out, nil
}
