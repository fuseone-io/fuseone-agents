package httpapi

import (
	"context"
	"errors"
	"fmt"

	"github.com/fuseone/agents/internal/admin"
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
//
// The row it authorized is the one the door then acts on, and no other: the
// answer carries the exact key, so a server removed and recreated under the
// same name in another scope meanwhile is not touched by an act that was
// authorized for the first.
func (s *Server) mcpConfigurer(ctx context.Context, name string) (
	domain.UserID, admin.MCPServerAt, *openapi.ForbiddenApplicationProblemPlusJSONResponse, error,
) {
	kind, stored, err := s.integrations.MCPServerStoredAt(ctx, name)
	if err != nil {
		return "", admin.MCPServerAt{}, nil, fmt.Errorf("find MCP server %s: %w", name, err)
	}
	caller, forbidden := s.configurerAt(ctx, kind, stored)
	return caller, admin.MCPServerAt{
		Kind: kind, Stored: stored, Audit: connectorRuntimeScope(kind, stored),
	}, forbidden, nil
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
// configured their own. A company or area with no kind is refused: written
// at the installation, it would be a server stored somewhere the caller did
// not ask for.
func connectorScopeOrInstallation(
	kind *openapi.ConnectorScopeKind, company, area *string,
) (settings.ScopeKind, domain.Scope, error) {
	if kind == nil {
		if company != nil || area != nil {
			return "", domain.Scope{}, errors.New("a company or area needs scopeKind")
		}
		return settings.ScopeInstallation, domain.Scope{}, nil
	}
	scope, err := connectorScope(*kind, company, area)
	if err != nil {
		return "", domain.Scope{}, err
	}
	return settings.ScopeKind(*kind), scope, nil
}

// errMCPServerRefused stops a read by name before the server's sealed document
// is opened: the caller may not configure the scope it is stored at.
var errMCPServerRefused = errors.New("the caller may not configure this server's scope")
