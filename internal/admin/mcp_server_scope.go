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

// placeTools gives each listed tool the runtime scope of the server that
// declares it (NT-019): what a run must be inside to reach it, as the worker's
// catalogue reads it (tools.Catalog.Scope). A server no row holds, one a --mcp
// flag connected, is the installation's.
func placeTools(ctx context.Context, q settings.DB, entries []domain.ToolEntry) error {
	scopes, err := serverScopes(ctx, q)
	if err != nil {
		return err
	}
	for i, e := range entries {
		scope, configured := scopes[e.Server]
		if !configured {
			scope = domain.Scope{Company: domain.Installation}
		}
		entries[i].Scope = scope
	}
	return nil
}

// rulingsInForce is the ruling each listed tool is under, chosen as the worker
// chooses it (domain.RulingsInForce): the platform's over the owner's, and the
// owner only at the scope of the tool's server, which placeTools gave each
// entry. Listed beside it are both rulings, as domain.RulingsOf reads them,
// for the screen.
func rulingsInForce(
	entries []domain.ToolEntry, rulings []domain.ToolClassification,
) (map[domain.ToolID]domain.ToolClassification, map[domain.ToolID][]domain.ToolRuling) {
	placed := make(map[domain.ToolID]domain.Scope, len(entries))
	for _, e := range entries {
		placed[e.ID] = e.Scope
	}
	scopeOf := func(id domain.ToolID) (domain.Scope, bool) {
		scope, ok := placed[id]
		return scope, ok
	}
	out := map[domain.ToolID]domain.ToolClassification{}
	for _, r := range domain.RulingsInForce(rulings, scopeOf) {
		out[r.Tool] = r
	}
	return out, domain.RulingsOf(rulings, scopeOf)
}

// MCPServerStoredAt is mcpServerAt, for a door that authorizes at the row's
// scope before it acts on a server it names.
func (i *Integrations) MCPServerStoredAt(ctx context.Context, name string) (settings.ScopeKind, domain.Scope, error) {
	return i.mcpServerAt(ctx, name)
}

// MCPServerAt is the exact row a door was authorized for, and the scope its
// act is recorded at. A door acts on this row and no other: the name is looked
// up once, before the authorization, and a row replaced since by another
// scope's server of the same name is not the one that was authorized.
type MCPServerAt struct {
	Kind   settings.ScopeKind
	Stored domain.Scope
	Audit  domain.Scope
}

// lockMCPServerAt holds the server's row at the exact key until the
// transaction ends, and answers whether it is switched on.
func lockMCPServerAt(ctx context.Context, tx settings.DB, at MCPServerAt, name string) (bool, error) {
	var enabled bool
	err := tx.QueryRow(ctx, `
		select enabled from settings
		where scope_kind = $1 and company_id = $2 and area_id = $3 and kind = $4 and name = $5
		for share`,
		string(at.Kind), string(at.Stored.Company), string(at.Stored.Area),
		string(settings.KindMCPServer), name,
	).Scan(&enabled)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, ErrMCPServerUnknown
	}
	if err != nil {
		return false, fmt.Errorf("admin: hold MCP server %s: %w", name, err)
	}
	return enabled, nil
}

// at is where a server is stored now, for a caller that authorized nothing.
func (i *Integrations) at(ctx context.Context, name string, audit domain.Scope) (MCPServerAt, error) {
	kind, stored, err := i.mcpServerAt(ctx, name)
	return MCPServerAt{Kind: kind, Stored: stored, Audit: audit}, err
}

/*
LockMCPServerName holds one lock per server name until the transaction ends.

A row lock cannot guard a name nobody holds: a removal that finds the name
nowhere clears what is left under it (personal credentials, observations), and
a creation of the name in another scope landing between that decision and the
clearing would lose what it just stored. Every write of a server and every
removal take this lock first, so the two take turns (NT-019).
*/
func LockMCPServerName(ctx context.Context, tx settings.DB, name string) error {
	if _, err := tx.Exec(ctx,
		`select pg_advisory_xact_lock(hashtextextended('mcp_server:' || $1, 0))`, name); err != nil {
		return fmt.Errorf("admin: lock the MCP server name %s: %w", name, err)
	}
	return nil
}

// nameHeldElsewhere answers ErrNameTaken when the name is a server's stored at
// another scope: names are unique across scopes (0108), and the write would
// meet that constraint only after judging everything else.
func nameHeldElsewhere(ctx context.Context, tx settings.DB, name string, kind settings.ScopeKind, stored domain.Scope) error {
	var elsewhere bool
	if err := tx.QueryRow(ctx, `
		select exists (select 1 from settings where kind = $1 and name = $2
			and not (scope_kind = $3 and company_id = $4 and area_id = $5))`,
		string(settings.KindMCPServer), name, string(kind), string(stored.Company), string(stored.Area),
	).Scan(&elsewhere); err != nil {
		return fmt.Errorf("admin: look for %s elsewhere: %w", name, err)
	}
	if elsewhere {
		return fmt.Errorf("%w: %s", settings.ErrNameTaken, name)
	}
	return nil
}
