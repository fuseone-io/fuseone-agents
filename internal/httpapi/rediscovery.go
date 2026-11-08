package httpapi

import (
	"context"
	"errors"
	"fmt"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

// RequestMCPRediscovery asks every worker holding a server to ask it again what
// it offers (#287). It takes what configuring the server takes: the answer
// changes the tools the installation offers, though not its configuration.
func (s *Server) RequestMCPRediscovery(
	ctx context.Context, req openapi.RequestMCPRediscoveryRequestObject,
) (openapi.RequestMCPRediscoveryResponseObject, error) {
	caller, forbidden := s.configurer(ctx)
	if forbidden != nil {
		return openapi.RequestMCPRediscovery403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: *forbidden,
		}, nil
	}
	if s.integrations == nil {
		return nil, errNoAdministration
	}
	generation, err := s.integrations.RequestMCPRediscovery(ctx, caller, adminScope, req.Name)
	switch {
	case errors.Is(err, admin.ErrMCPServerUnknown):
		return openapi.RequestMCPRediscovery404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse: notFound(req.Name),
		}, nil
	case errors.Is(err, admin.ErrMCPServerDisabled):
		return openapi.RequestMCPRediscovery400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(
				invalid("the server is switched off, so no worker holds it")),
		}, nil
	case err != nil:
		return nil, fmt.Errorf("request rediscovery: %w", err)
	}
	return openapi.RequestMCPRediscovery202JSONResponse{Generation: generation}, nil
}

// GetMCPRediscovery answers the latest request and each worker's newest answer.
func (s *Server) GetMCPRediscovery(
	ctx context.Context, req openapi.GetMCPRediscoveryRequestObject,
) (openapi.GetMCPRediscoveryResponseObject, error) {
	if _, forbidden := s.configurer(ctx); forbidden != nil {
		return openapi.GetMCPRediscovery403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: *forbidden,
		}, nil
	}
	if s.integrations == nil {
		return nil, errNoAdministration
	}
	status, err := s.integrations.MCPRediscovery(ctx, req.Name)
	if errors.Is(err, admin.ErrMCPServerUnknown) {
		return openapi.GetMCPRediscovery404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse: notFound(req.Name),
		}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get rediscovery: %w", err)
	}
	return openapi.GetMCPRediscovery200JSONResponse(rediscoveryFrom(status)), nil
}

func rediscoveryFrom(status domain.MCPRediscoveryStatus) openapi.MCPRediscovery {
	out := openapi.MCPRediscovery{
		Server: status.Server, Generation: status.Generation,
		// Empty and never null: the schema says array.
		Receipts: make([]openapi.MCPRediscoveryReceipt, 0, len(status.Receipts)),
	}
	if status.Generation > 0 {
		out.RequestedBy = ptr(string(status.RequestedBy))
		out.RequestedAt = ptr(status.RequestedAt)
	}
	for _, r := range status.Receipts {
		out.Receipts = append(out.Receipts, openapi.MCPRediscoveryReceipt{
			Worker: r.Worker, Generation: r.Generation, Succeeded: r.Succeeded, At: r.At,
		})
	}
	return out
}
