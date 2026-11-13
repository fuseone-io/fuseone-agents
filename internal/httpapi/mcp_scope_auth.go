package httpapi

import (
	"context"
	"fmt"

	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/settings"
)

/*
Who may configure an MCP server, and who sees one (NT-019, phase 2 writers).

The rule is the connector instances': writing needs provider:write at the
row's scope, or at the platform's administration, and the installation's
rows keep today's check. A server is found by its name, unique across scopes
(0108), so a door that names one reads where it lives first. An area's server
is remote (decided 2026-11-13): a local one runs on the platform's worker.
*/

// mcpServerStored is where the row a runtime scope came from is stored.
func mcpServerStored(runtime domain.Scope) (settings.ScopeKind, domain.Scope) {
	switch {
	case runtime == (domain.Scope{}) || runtime.Company == domain.Installation:
		return settings.ScopeInstallation, domain.Scope{}
	case runtime.Area == "":
		return settings.ScopeCompany, runtime
	default:
		return settings.ScopeArea, runtime
	}
}

// mcpConfigurer is configurerAt for the server a door names. A name no row
// holds is the installation's to write, as a new server is by default.
func (s *Server) mcpConfigurer(ctx context.Context, name string) (
	domain.UserID, domain.Scope, *openapi.ForbiddenApplicationProblemPlusJSONResponse, error,
) {
	kind, stored, err := s.integrations.MCPServerStoredAt(ctx, name)
	if err != nil {
		return "", domain.Scope{}, nil, fmt.Errorf("find MCP server %s: %w", name, err)
	}
	caller, forbidden := s.configurerAt(ctx, kind, stored)
	return caller, connectorRuntimeScope(kind, stored), forbidden, nil
}

// mcpListingCaller is who may read the integrations listing: a reader of the
// platform's, as before, or whoever configures in some scope (rule 4).
func mcpListingCaller(ctx context.Context) (domain.Principal, bool) {
	p, _ := auth.PrincipalFrom(ctx)
	return p, p.Can(domain.PermToolRead, adminScope) || len(p.ScopesFor(permConfigure)) > 0
}

// mcpServerListing places one configured server in the caller's listing.
func mcpServerListing(p domain.Principal, srv domain.MCPServer) (listed, editable bool) {
	kind, stored := mcpServerStored(srv.Scope)
	if p.Can(domain.PermToolRead, adminScope) {
		return true, mayConfigure(p, kind, stored)
	}
	return rowListing(p, kind, stored)
}

// connectorScopeOrInstallation reads the scope a write names, the
// installation's when it names none, as every server was before areas
// configured their own.
func connectorScopeOrInstallation(
	kind *openapi.ConnectorScopeKind, company, area *string,
) (settings.ScopeKind, domain.Scope, error) {
	if kind == nil {
		return settings.ScopeInstallation, domain.Scope{}, nil
	}
	scope, err := connectorScope(*kind, company, area)
	if err != nil {
		return "", domain.Scope{}, err
	}
	return settings.ScopeKind(*kind), scope, nil
}
