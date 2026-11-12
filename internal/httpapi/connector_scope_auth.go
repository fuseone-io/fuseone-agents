package httpapi

import (
	"context"

	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/connectortools"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/settings"
)

/*
Who may configure a connector instance, and who sees one (NT-019, phase 1).

A row has three scopes. It is stored as installation, company or area; it runs
for every run its runtime scope contains; and its writer must hold
provider:write at its authorization scope. The installation's rows keep today's
check, at the platform's administration, so nobody who configures them loses
that right. A company's or an area's row is written with the permission there,
or by the platform's administration, which governs every area. adminScope does
not contain acme/cx by Scope.Contains, so that is a second condition, not the
hierarchy.
*/

// connectorRuntimeScope is the scope a run must be inside to use the row.
func connectorRuntimeScope(kind settings.ScopeKind, stored domain.Scope) domain.Scope {
	if kind == settings.ScopeInstallation {
		return domain.Scope{Company: domain.Installation}
	}
	return stored
}

// mayConfigure answers rule 2 for one row: the permission at its
// authorization scope.
func mayConfigure(p domain.Principal, kind settings.ScopeKind, stored domain.Scope) bool {
	if p.Can(permConfigure, adminScope) {
		return true
	}
	if kind == settings.ScopeInstallation {
		return false
	}
	return p.Can(permConfigure, stored)
}

// configurerAt is configurer for a row of a known scope.
func (s *Server) configurerAt(
	ctx context.Context, kind settings.ScopeKind, stored domain.Scope,
) (domain.UserID, *openapi.ForbiddenApplicationProblemPlusJSONResponse) {
	p, _ := auth.PrincipalFrom(ctx)
	if !mayConfigure(p, kind, stored) {
		body := forbidden(permConfigure, connectorRuntimeScope(kind, stored))
		return "", &body
	}
	return p.ID, nil
}

// connectorListing answers rule 4 for one row: listed when the caller's area
// inherits it or the caller governs it, and editable when rule 2 lets them
// write it. Nothing of a sibling area is listed.
func connectorListing(p domain.Principal, instance connectortools.ConfiguredInstance) (listed, editable bool) {
	editable = mayConfigure(p, instance.ScopeKind, instance.Scope)
	if editable {
		return true, true
	}
	runtime := connectorRuntimeScope(instance.ScopeKind, instance.Scope)
	for _, held := range p.ScopesFor(permConfigure) {
		if runtime.Contains(held) {
			return true, false
		}
	}
	return false, false
}

// configuresSomewhere refuses a caller with provider:write in no scope at all,
// before a body is read: what a request may not do is not explained to them
// through its validation errors.
func configuresSomewhere(ctx context.Context) *openapi.ForbiddenApplicationProblemPlusJSONResponse {
	p, _ := auth.PrincipalFrom(ctx)
	if len(p.ScopesFor(permConfigure)) == 0 {
		body := forbidden(permConfigure, adminScope)
		return &body
	}
	return nil
}
