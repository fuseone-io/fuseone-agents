package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

// Rates is the installation's price list, declared here by the consumer.
type Rates interface {
	Prices(ctx context.Context) ([]admin.ModelPrice, error)
	PutPrice(ctx context.Context, by domain.UserID, scope domain.Scope, price admin.ModelPrice) error
	DeletePrice(ctx context.Context, by domain.UserID, scope domain.Scope, provider, model string) error
}

// WithRates wires the price list.
func (s *Server) WithRates(r Rates) *Server {
	s.rates = r
	return s
}

func (s *Server) ListPrices(
	ctx context.Context, _ openapi.ListPricesRequestObject,
) (openapi.ListPricesResponseObject, error) {
	// Read with tool:read rather than write access: an author looking at a
	// cost figure that says zero deserves to find out that nobody set a rate,
	// without needing permission to change one.
	if resp := s.refuse(ctx, domain.PermToolRead); resp != nil {
		return openapi.ListPrices403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: *resp,
		}, nil
	}
	if s.rates == nil {
		return openapi.ListPrices200JSONResponse{Items: []openapi.ModelPrice{}}, nil
	}

	stored, err := s.rates.Prices(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]openapi.ModelPrice, 0, len(stored))
	for _, p := range stored {
		items = append(items, priceFrom(p))
	}
	return openapi.ListPrices200JSONResponse{Items: items}, nil
}

func (s *Server) PutPrice(
	ctx context.Context, req openapi.PutPriceRequestObject,
) (openapi.PutPriceResponseObject, error) {
	if resp := s.refuse(ctx, domain.PermBudgetWrite); resp != nil {
		return openapi.PutPrice403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: *resp,
		}, nil
	}
	if s.rates == nil || req.Body == nil {
		return nil, errNoAdministration
	}

	err := s.rates.PutPrice(ctx, callerOf(ctx), adminScope, priceInto(*req.Body))
	if errors.Is(err, admin.ErrNoModel) {
		return openapi.PutPrice400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(
				problem(http.StatusBadRequest, "Tarifa incompleta", err.Error())),
		}, nil
	}
	if err != nil {
		return nil, err
	}
	return openapi.PutPrice204Response{}, nil
}

func (s *Server) DeletePrice(
	ctx context.Context, req openapi.DeletePriceRequestObject,
) (openapi.DeletePriceResponseObject, error) {
	if resp := s.refuse(ctx, domain.PermBudgetWrite); resp != nil {
		return openapi.DeletePrice403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: *resp,
		}, nil
	}
	if s.rates == nil {
		return nil, errNoAdministration
	}
	if err := s.rates.DeletePrice(ctx, callerOf(ctx), adminScope, req.Provider, req.Model); err != nil {
		return nil, err
	}
	return openapi.DeletePrice204Response{}, nil
}

func priceFrom(p admin.ModelPrice) openapi.ModelPrice {
	return openapi.ModelPrice{
		Provider: p.Provider, Model: p.Model,
		InputMicros: ptr(p.InputMicros), OutputMicros: ptr(p.OutputMicros),
		CacheReadMicros: ptr(p.CacheReadMicros), CacheWriteMicros: ptr(p.CacheWriteMicros),
	}
}

func priceInto(in openapi.ModelPrice) admin.ModelPrice {
	return admin.ModelPrice{
		Provider: in.Provider, Model: in.Model,
		InputMicros: valueOr(in.InputMicros), OutputMicros: valueOr(in.OutputMicros),
		CacheReadMicros: valueOr(in.CacheReadMicros), CacheWriteMicros: valueOr(in.CacheWriteMicros),
	}
}
