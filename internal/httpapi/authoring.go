package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/fuseone/agents/internal/authoring"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

// Authoring is the interview's model choice, declared here by the consumer.
type Authoring interface {
	Current(ctx context.Context) (authoring.Choice, error)
	Choose(ctx context.Context, choice authoring.Choice, by domain.UserID) error
	Disable(ctx context.Context, by domain.UserID) error
}

// WithAuthoring wires the choice.
func (s *Server) WithAuthoring(a Authoring) *Server {
	s.authoring = a
	return s
}

func (s *Server) GetAuthoring(
	ctx context.Context, _ openapi.GetAuthoringRequestObject,
) (openapi.GetAuthoringResponseObject, error) {
	if resp := s.refuse(ctx, domain.PermProviderWrite); resp != nil {
		return openapi.GetAuthoring403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: *resp,
		}, nil
	}
	if s.authoring == nil {
		return openapi.GetAuthoring200JSONResponse{Enabled: false}, nil
	}

	choice, err := s.authoring.Current(ctx)
	if err != nil {
		return nil, err
	}
	return openapi.GetAuthoring200JSONResponse(choiceFrom(choice)), nil
}

func (s *Server) SetAuthoring(
	ctx context.Context, req openapi.SetAuthoringRequestObject,
) (openapi.SetAuthoringResponseObject, error) {
	if resp := s.refuse(ctx, domain.PermProviderWrite); resp != nil {
		return openapi.SetAuthoring403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: *resp,
		}, nil
	}
	if s.authoring == nil || req.Body == nil {
		return nil, errNoAdministration
	}

	// Switching it off keeps the provider it used, so turning it back on does
	// not mean answering the same question twice.
	if !req.Body.Enabled {
		if err := s.authoring.Disable(ctx, callerOf(ctx)); err != nil {
			return nil, err
		}
		return openapi.SetAuthoring204Response{}, nil
	}

	err := s.authoring.Choose(ctx, authoring.Choice{
		Provider:    req.Body.Provider,
		Model:       req.Body.Model,
		Effort:      valueOr(req.Body.Effort),
		DailyMicros: valueOr(req.Body.DailyMicros),
	}, callerOf(ctx))
	if errors.Is(err, authoring.ErrNoProvider) || errors.Is(err, authoring.ErrNoCeiling) {
		return openapi.SetAuthoring400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(
				problem(http.StatusBadRequest, "Provedor desconhecido", err.Error())),
		}, nil
	}
	if err != nil {
		return openapi.SetAuthoring400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(
				problem(http.StatusBadRequest, "Escolha inválida", err.Error())),
		}, nil
	}
	return openapi.SetAuthoring204Response{}, nil
}

func choiceFrom(c authoring.Choice) openapi.AuthoringChoice {
	return openapi.AuthoringChoice{
		Provider:    c.Provider,
		Model:       c.Model,
		Effort:      ptr(c.Effort),
		DailyMicros: ptr(c.DailyMicros),
		Enabled:     c.Enabled,
	}
}
