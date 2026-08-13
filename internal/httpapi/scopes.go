package httpapi

import (
	"context"
	"errors"
	"strings"

	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/scope"
)

// Areas is the registry of declared areas, declared here by the consumer.
type Areas interface {
	List(ctx context.Context, visible []domain.Scope) ([]domain.RegisteredScope, error)
	Put(ctx context.Context, company domain.CompanyID, typed, label string, by domain.UserID) (domain.RegisteredScope, error)
	Delete(ctx context.Context, company domain.CompanyID, area domain.AreaID) error
}

// WithAreas wires the registry.
func (s *Server) WithAreas(areas Areas) *Server {
	s.areas = areas
	return s
}

// ListScopes answers with the areas the caller reaches.
//
// Read by anyone who can see an agent, which is every role: this list is what
// a context switcher offers, and a person who cannot see the areas they work
// in cannot choose one. Writing is a different matter entirely.
func (s *Server) ListScopes(
	ctx context.Context, _ openapi.ListScopesRequestObject,
) (openapi.ListScopesResponseObject, error) {
	visible := auth.VisibleScopes(ctx, domain.PermAgentRead)
	if len(visible) == 0 {
		return openapi.ListScopes403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: forbidden(domain.PermAgentRead, domain.Scope{}),
		}, nil
	}
	if s.areas == nil {
		return openapi.ListScopes200JSONResponse{Items: []openapi.RegisteredScope{}}, nil
	}

	registered, err := s.areas.List(ctx, visible)
	if err != nil {
		return nil, err
	}

	items := make([]openapi.RegisteredScope, 0, len(registered))
	for _, r := range registered {
		items = append(items, registeredFrom(r))
	}
	return openapi.ListScopes200JSONResponse{Items: items}, nil
}

// RegisterScope declares an area, or relabels the one a name folds onto.
func (s *Server) RegisterScope(
	ctx context.Context, req openapi.RegisterScopeRequestObject,
) (openapi.RegisterScopeResponseObject, error) {
	if resp := s.refuse(ctx, domain.PermScopeWrite); resp != nil {
		return openapi.RegisterScope403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: *resp,
		}, nil
	}
	if s.areas == nil || req.Body == nil {
		return nil, errNoAdministration
	}

	label := req.Body.Name
	if req.Body.Label != nil && *req.Body.Label != "" {
		label = *req.Body.Label
	}

	registered, err := s.areas.Put(
		ctx, domain.CompanyID(req.Body.Company), req.Body.Name, label, callerOf(ctx))
	if err != nil {
		return openapi.RegisterScope400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(
				invalid(err.Error())),
		}, nil
	}
	return openapi.RegisterScope200JSONResponse(registeredFrom(registered)), nil
}

// DeleteScope withdraws an area from what is offered for new work.
func (s *Server) DeleteScope(
	ctx context.Context, req openapi.DeleteScopeRequestObject,
) (openapi.DeleteScopeResponseObject, error) {
	if resp := s.refuse(ctx, domain.PermScopeWrite); resp != nil {
		return openapi.DeleteScope403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: *resp,
		}, nil
	}
	if s.areas == nil {
		return nil, errNoAdministration
	}

	company, area, ok := strings.Cut(req.Scope, "/")
	if !ok || company == "" || area == "" {
		return openapi.DeleteScope404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse: notFound(req.Scope),
		}, nil
	}

	err := s.areas.Delete(ctx, domain.CompanyID(company), domain.AreaID(area))
	if errors.Is(err, scope.ErrNoArea) {
		return openapi.DeleteScope404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse: notFound(req.Scope),
		}, nil
	}
	if err != nil {
		return nil, err
	}
	return openapi.DeleteScope204Response{}, nil
}

func registeredFrom(r domain.RegisteredScope) openapi.RegisteredScope {
	return openapi.RegisteredScope{
		Company:   string(r.Scope.Company),
		Area:      string(r.Scope.Area),
		Label:     ptr(r.Label),
		CreatedAt: &r.CreatedAt,
		CreatedBy: ptr(string(r.CreatedBy)),
	}
}
