package httpapi

import (
	"context"
	"fmt"

	"github.com/fuseone/agents/internal/audit"
	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

// WithAudit wires the merged trail.
func (s *Server) WithAudit(trail audit.Reader) *Server {
	s.audit = trail
	return s
}

// ListAudit answers with what happened, across both records.
//
// Narrowed to what the caller may see rather than refused when they name no
// scope: asking "what happened" should answer with their areas, not with a
// permission error about one they never mentioned (NF-06).
func (s *Server) ListAudit(
	ctx context.Context, req openapi.ListAuditRequestObject,
) (openapi.ListAuditResponseObject, error) {
	visible := auth.VisibleScopes(ctx, domain.PermAuditRead)
	if len(visible) == 0 {
		return openapi.ListAudit403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: forbidden(domain.PermAuditRead, domain.Scope{}),
		}, nil
	}
	if s.audit == nil {
		return openapi.ListAudit200JSONResponse{Items: []openapi.AuditEntry{}}, nil
	}

	filter := audit.Filter{Scopes: visible}
	if named := scopeFrom(req.Params.Company, req.Params.Area); named != (domain.Scope{}) {
		// A named scope is checked in that scope rather than intersected
		// silently: somebody asking about marketing should be told no, not
		// handed an empty page they will read as "nothing happened".
		if err := auth.Require(ctx, domain.PermAuditRead, named); err != nil {
			return openapi.ListAudit403ApplicationProblemPlusJSONResponse{
				ForbiddenApplicationProblemPlusJSONResponse: forbidden(domain.PermAuditRead, named),
			}, nil
		}
		filter.Scopes = []domain.Scope{named}
	}
	if req.Params.Since != nil {
		filter.Since = *req.Params.Since
	}
	if req.Params.Until != nil {
		filter.Until = *req.Params.Until
	}
	if req.Params.Actor != nil {
		filter.Actor = *req.Params.Actor
	}
	if req.Params.Source != nil {
		filter.Sources = []audit.Source{audit.Source(*req.Params.Source)}
	}

	entries, err := s.audit.Read(ctx, filter, limitOf(req.Params.Limit))
	if err != nil {
		return nil, fmt.Errorf("read audit trail: %w", err)
	}

	items := make([]openapi.AuditEntry, 0, len(entries))
	for _, entry := range entries {
		items = append(items, auditEntryFrom(entry))
	}
	return openapi.ListAudit200JSONResponse{Items: items}, nil
}

func auditEntryFrom(entry audit.Entry) openapi.AuditEntry {
	out := openapi.AuditEntry{
		At:     entry.At,
		Source: openapi.AuditEntrySource(entry.Source),
		Actor:  entry.Actor,
		Verb:   entry.Verb,
		Target: entry.Target,
		Scope: &openapi.Scope{
			Company: string(entry.Scope.Company), Area: string(entry.Scope.Area),
		},
	}
	if len(entry.Detail) > 0 {
		out.Detail = &entry.Detail
	}
	if entry.RunID != "" {
		out.RunId = ptr(string(entry.RunID))
		out.Seq = ptr(entry.Seq)
	}
	// Absent rather than empty: an administrative entry has nothing to show
	// here and says so by omission rather than by an empty promise.
	if entry.Hash != "" {
		out.Hash = ptr(entry.Hash)
	}
	return out
}

// scopeFrom reads a named scope out of the query, or the zero scope.
func scopeFrom(company, area *string) domain.Scope {
	var scope domain.Scope
	if company != nil {
		scope.Company = domain.CompanyID(*company)
	}
	if area != nil {
		scope.Area = domain.AreaID(*area)
	}
	return scope
}
