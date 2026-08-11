package httpapi

import (
	"context"

	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

// narrow bounds a listing to what the caller may see.
//
// A named scope is checked in that scope; an unnamed one is answered with the
// caller's own. The second half is why this is a filter rather than a refusal:
// asking "which runs are there" should answer with theirs, not with a
// permission error naming a scope they never mentioned (PRD NF-06).
//
// It returns false when the caller holds the permission nowhere, which is a
// refusal rather than an empty page — the two mean different things to
// somebody trying to work out why a screen is blank.
func narrow(ctx context.Context, filter domain.RunFilter, perm domain.Permission) (domain.RunFilter, bool) {
	if filter.Scope.Company != "" || filter.Scope.Area != "" {
		if err := auth.Require(ctx, perm, filter.Scope); err != nil {
			return filter, false
		}
		filter.Scopes = []domain.Scope{filter.Scope}
		filter.Scope = domain.Scope{}
		return filter, true
	}

	visible := auth.VisibleScopes(ctx, perm)
	if len(visible) == 0 {
		return filter, false
	}
	filter.Scopes = visible
	return filter, true
}

// mayRead reports whether the caller may read a resource in a scope.
func mayRead(ctx context.Context, perm domain.Permission, scope domain.Scope) bool {
	return auth.Require(ctx, perm, scope) == nil
}

// scopeParams reads the scope a caller asked for.
func scopeParams(company, area *string) domain.Scope {
	var scope domain.Scope
	if company != nil {
		scope.Company = domain.CompanyID(*company)
	}
	if area != nil {
		scope.Area = domain.AreaID(*area)
	}
	return scope
}

func forbiddenListRuns(perm domain.Permission, scope domain.Scope) openapi.ListRuns403ApplicationProblemPlusJSONResponse {
	return openapi.ListRuns403ApplicationProblemPlusJSONResponse{
		ForbiddenApplicationProblemPlusJSONResponse: forbidden(perm, scope),
	}
}
