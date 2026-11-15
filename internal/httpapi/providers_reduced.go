package httpapi

import (
	"context"
	"errors"
	"fmt"

	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

/*
ListProviders is GET /providers: the providers an author picks from (NT-019,
rule 5, phase 2 writers part B2a).

Names, kinds, models and whether each is usable, and nothing else: the
configuration listing carries addresses and is not for authors. It reads on
tool:read, as the tool catalogue does, and is asked for one target scope by
the same rule (readerReach).
*/
func (s *Server) ListProviders(ctx context.Context, req openapi.ListProvidersRequestObject) (openapi.ListProvidersResponseObject, error) {
	if len(auth.VisibleScopes(ctx, domain.PermToolRead)) == 0 {
		return openapi.ListProviders403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: forbidden(domain.PermToolRead, domain.Scope{}),
		}, nil
	}
	keep, err := readerReach(ctx, req.Params.Scope)
	switch {
	case errors.Is(err, errTargetHidden):
		return openapi.ListProviders404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse: notFound("scope " + *req.Params.Scope),
		}, nil
	case err != nil:
		return openapi.ListProviders400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(
				invalid(err.Error())),
		}, nil
	}
	page := openapi.ListProviders200JSONResponse{Items: []openapi.ProviderChoice{}}
	if s.integrations == nil {
		return page, nil
	}
	providers, err := s.integrations.Providers(ctx)
	if err != nil {
		return nil, fmt.Errorf("list providers: %w", err)
	}
	target := providerTarget(req.Params.Scope)
	for _, p := range providers {
		if keep(p.Scope) {
			page.Items = append(page.Items, s.providerChoice(p, target))
		}
	}
	return page, nil
}

// providerChoice is a provider as an author picks it, built field by field so
// nothing the configuration carries rides along.
func (s *Server) providerChoice(p domain.ModelProvider, target *domain.Scope) openapi.ProviderChoice {
	return openapi.ProviderChoice{
		Name: p.Name, Kind: openapi.ProviderChoiceKind(p.Kind),
		Models: append([]string{}, p.Models...), Usable: s.usable(p, target),
	}
}

// providerTarget is the scope asked for, already judged by readerReach, or
// nil when none was: then each provider is judged at its own scope.
func providerTarget(raw *string) *domain.Scope {
	if raw == nil {
		return nil
	}
	target, err := parseTarget(*raw)
	if err != nil {
		return nil
	}
	return &target
}

// usable is whether this process built the provider and would ask it
// (Registry.Usable): switched on, its credential opened, its address whole.
// Unwired, as on a laptop, nothing says what was built, and only a provider
// switched off is known not to be.
func (s *Server) usable(p domain.ModelProvider, target *domain.Scope) bool {
	if s.providerNames == nil {
		return p.Enabled
	}
	at := p.Scope
	if target != nil {
		at = *target
	}
	return p.Enabled && s.providerNames.Usable(p.Name, at)
}
