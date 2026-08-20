package httpapi

import (
	"context"
	"testing"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
)

type ratesSpy struct {
	prices  []admin.ModelPrice
	put     admin.ModelPrice
	deleted string
}

func (r *ratesSpy) Prices(context.Context) ([]admin.ModelPrice, error) {
	return r.prices, nil
}

func (r *ratesSpy) PutPrice(_ context.Context, _ domain.UserID, _ domain.Scope, p admin.ModelPrice) error {
	r.put = p
	return nil
}

func (r *ratesSpy) DeletePrice(
	_ context.Context, _ domain.UserID, _ domain.Scope, provider, model string,
) error {
	r.deleted = provider + "/" + model
	return nil
}

func TestListPrices_includesBundledMarketDefaults(t *testing.T) {
	t.Parallel()

	resp, err := NewServer(ledger.NewMemory(), "test").WithRates(&ratesSpy{}).
		ListPrices(as(domain.RoleAuthor), openapi.ListPricesRequestObject{})
	if err != nil {
		t.Fatalf("ListPrices: %v", err)
	}
	page, ok := resp.(openapi.ListPrices200JSONResponse)
	if !ok {
		t.Fatalf("response = %T", resp)
	}
	price := findPrice(t, page.Items, "anthropic", "claude-opus-5")
	if price.Source == nil || *price.Source != openapi.MarketDefault {
		t.Fatalf("source = %v, want market default", price.Source)
	}
	if price.InputMicros == nil || *price.InputMicros != 2_500_000 {
		t.Fatalf("input = %v, want the bundled default", price.InputMicros)
	}
	if price.Currency == nil || *price.Currency != "USD" {
		t.Fatalf("currency = %v, want USD", price.Currency)
	}
	if price.SourceUrl == nil {
		t.Fatal("market defaults should name the source page")
	}
}

func TestListPrices_includesMarketDefaultsEvenWithoutAConfiguredStore(t *testing.T) {
	t.Parallel()

	resp, err := NewServer(ledger.NewMemory(), "test").
		ListPrices(as(domain.RoleAuthor), openapi.ListPricesRequestObject{})
	if err != nil {
		t.Fatalf("ListPrices: %v", err)
	}
	page, ok := resp.(openapi.ListPrices200JSONResponse)
	if !ok {
		t.Fatalf("response = %T", resp)
	}
	_ = findPrice(t, page.Items, "anthropic", "claude-opus-5")
}

func TestListPrices_configuredRateOverridesTheMarketDefault(t *testing.T) {
	t.Parallel()
	rates := &ratesSpy{prices: []admin.ModelPrice{{
		Provider: "anthropic", Model: "claude-opus-5",
		InputMicros: 123, Source: "configured",
	}}}

	resp, err := NewServer(ledger.NewMemory(), "test").WithRates(rates).
		ListPrices(as(domain.RoleAuthor), openapi.ListPricesRequestObject{})
	if err != nil {
		t.Fatalf("ListPrices: %v", err)
	}
	page, ok := resp.(openapi.ListPrices200JSONResponse)
	if !ok {
		t.Fatalf("response = %T", resp)
	}

	var count int
	for _, price := range page.Items {
		if price.Provider == "anthropic" && price.Model == "claude-opus-5" {
			count++
			if price.Source == nil || *price.Source != openapi.Configured {
				t.Fatalf("source = %v, want configured", price.Source)
			}
			if price.InputMicros == nil || *price.InputMicros != 123 {
				t.Fatalf("input = %v, want configured override", price.InputMicros)
			}
		}
	}
	if count != 1 {
		t.Fatalf("got %d rows for anthropic/claude-opus-5, want one", count)
	}
}

func findPrice(
	t *testing.T, prices []openapi.ModelPrice, provider, model string,
) openapi.ModelPrice {
	t.Helper()
	for _, price := range prices {
		if price.Provider == provider && price.Model == model {
			return price
		}
	}
	t.Fatalf("missing price %s/%s in %+v", provider, model, prices)
	return openapi.ModelPrice{}
}
