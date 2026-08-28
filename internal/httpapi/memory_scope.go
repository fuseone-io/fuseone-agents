package httpapi

import (
	"context"
	"fmt"
	"strings"

	memstore "github.com/fuseone/agents/internal/memory"

	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

// Which scopes a request may see, and what it asked for.
//
// Read and write ask different questions. A list narrows to everything the
// caller can read and answers with less; a write names one scope and is refused
// if the caller cannot publish there.

func memoryReadableScopes(
	ctx context.Context, params openapi.ListMemoryAssertionsParams,
) ([]domain.Scope, *openapi.ForbiddenApplicationProblemPlusJSONResponse) {
	requested := memoryScopeParam(params.Company, params.Area)
	if requested.Company != "" || requested.Area != "" {
		if err := auth.Require(ctx, domain.PermAgentRead, requested); err != nil {
			body := forbidden(domain.PermAgentRead, requested)
			return nil, &body
		}
		return []domain.Scope{requested}, nil
	}
	visible := auth.VisibleScopes(ctx, domain.PermAgentRead)
	if len(visible) == 0 {
		body := forbidden(domain.PermAgentRead, domain.Scope{})
		return nil, &body
	}
	return visible, nil
}

func memoryFilter(scopes []domain.Scope, params openapi.ListMemoryAssertionsParams) memstore.Filter {
	filter := memstore.Filter{Scopes: scopes, Limit: limitOf(params.Limit)}
	if params.AgentId != nil {
		filter.AgentID = domain.AgentID(strings.TrimSpace(*params.AgentId))
	}
	if params.Status != nil {
		filter.Status = domain.MemoryStatus(*params.Status)
	}
	if params.Q != nil {
		filter.Search = *params.Q
	}
	return filter
}

func suggestionReadableScopes(
	ctx context.Context, params openapi.ListMemorySuggestionsParams,
) ([]domain.Scope, *openapi.ForbiddenApplicationProblemPlusJSONResponse) {
	requested := memoryScopeParam(params.Company, params.Area)
	if requested.Company != "" || requested.Area != "" {
		if err := auth.Require(ctx, domain.PermAgentRead, requested); err != nil {
			body := forbidden(domain.PermAgentRead, requested)
			return nil, &body
		}
		return []domain.Scope{requested}, nil
	}
	visible := auth.VisibleScopes(ctx, domain.PermAgentRead)
	if len(visible) == 0 {
		body := forbidden(domain.PermAgentRead, domain.Scope{})
		return nil, &body
	}
	return visible, nil
}

func suggestionFilter(scopes []domain.Scope, params openapi.ListMemorySuggestionsParams) memstore.SuggestionFilter {
	filter := memstore.SuggestionFilter{Scopes: scopes, Limit: limitOf(params.Limit)}
	if params.AgentId != nil {
		filter.AgentID = domain.AgentID(strings.TrimSpace(*params.AgentId))
	}
	if params.Status != nil {
		filter.Status = domain.MemorySuggestionStatus(*params.Status)
	}
	if params.Q != nil {
		filter.Search = *params.Q
	}
	return filter
}

func memoryScopeParam(company *openapi.CompanyScope, area *openapi.Area) domain.Scope {
	var scope domain.Scope
	if company != nil {
		scope.Company = domain.CompanyID(*company)
	}
	if area != nil {
		scope.Area = domain.AreaID(*area)
	}
	return scope
}

func inputScope(company, area string) (domain.Scope, error) {
	scope := domain.Scope{Company: domain.CompanyID(strings.TrimSpace(company)), Area: domain.AreaID(strings.TrimSpace(area))}
	if !scope.Valid() || scope.Company == domain.Installation {
		return domain.Scope{}, fmt.Errorf("memory scope must name a company and area")
	}
	return scope, nil
}
