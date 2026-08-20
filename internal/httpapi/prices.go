package httpapi

import (
	"context"
	"errors"
	"sort"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/model"
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
	var stored []admin.ModelPrice
	if s.rates != nil {
		var err error
		stored, err = s.rates.Prices(ctx)
		if err != nil {
			return nil, err
		}
	}
	items := make([]openapi.ModelPrice, 0, len(stored)+len(model.MarketPrices()))
	seen := map[string]struct{}{}
	for _, p := range stored {
		seen[p.Provider+"/"+p.Model] = struct{}{}
		items = append(items, priceFrom(p))
	}
	for _, p := range model.MarketPrices() {
		if _, ok := seen[p.Provider+"/"+p.Model]; ok {
			continue
		}
		items = append(items, marketPriceFrom(p))
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Provider == items[j].Provider {
			return items[i].Model < items[j].Model
		}
		return items[i].Provider < items[j].Provider
	})
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
				invalid(err.Error())),
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
		Source:    ptr(sourceOf(p.Source)),
		Currency:  ptrNonEmpty(p.Currency),
		SourceUrl: ptrNonEmpty(p.SourceURL), SourceUpdatedAt: ptrNonEmpty(p.SourceUpdatedAt),
	}
}

func marketPriceFrom(p model.MarketPrice) openapi.ModelPrice {
	return openapi.ModelPrice{
		Provider: p.Provider, Model: p.Model,
		InputMicros: ptr(p.Prices.InputMicros), OutputMicros: ptr(p.Prices.OutputMicros),
		CacheReadMicros:  ptr(p.Prices.CacheReadMicros),
		CacheWriteMicros: ptr(p.Prices.CacheWriteMicros),
		Source:           ptr(openapi.MarketDefault),
		Currency:         ptrNonEmpty(p.Currency),
		SourceUrl:        ptrNonEmpty(p.SourceURL), SourceUpdatedAt: ptrNonEmpty(p.SourceUpdatedAt),
	}
}

func priceInto(in openapi.ModelPrice) admin.ModelPrice {
	return admin.ModelPrice{
		Provider: in.Provider, Model: in.Model,
		InputMicros: valueOr(in.InputMicros), OutputMicros: valueOr(in.OutputMicros),
		CacheReadMicros: valueOr(in.CacheReadMicros), CacheWriteMicros: valueOr(in.CacheWriteMicros),
	}
}

func sourceOf(source string) openapi.ModelPriceSource {
	if source == model.PriceSourceMarketDefault {
		return openapi.MarketDefault
	}
	return openapi.Configured
}

func ptrNonEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
