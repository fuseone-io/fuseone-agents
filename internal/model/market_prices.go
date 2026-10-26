package model

import (
	"maps"
	"slices"
)

const (
	PriceSourceConfigured    = "configured"
	PriceSourceMarketDefault = "market_default"
)

// MarketPrice is a public list price the platform knows about.
//
// These are defaults, not an invoice and not a ledger rate. They are public
// reference values in their own currency, shown so an operator has something
// to start from, but Cost.Micros stays zero until a configured installation
// rate exists.
type MarketPrice struct {
	Provider        string
	Model           string
	Prices          Prices
	Currency        string
	SourceURL       string
	SourceUpdatedAt string
}

var marketPrices = map[string]MarketPrice{
	"anthropic/claude-opus-5": {
		Provider: "anthropic", Model: "claude-opus-5",
		Prices: Prices{
			InputMicros:        5_000_000,
			OutputMicros:       25_000_000,
			CacheReadMicros:    500_000,
			CacheWriteMicros:   6_250_000,
			CacheWrite1hMicros: 10_000_000,
		},
		Currency:        "USD",
		SourceURL:       "https://platform.claude.com/docs/en/about-claude/pricing",
		SourceUpdatedAt: "2026-10-26",
	},
	"anthropic/claude-sonnet-5": {
		Provider: "anthropic", Model: "claude-sonnet-5",
		Prices: Prices{
			InputMicros:        2_000_000,
			OutputMicros:       10_000_000,
			CacheReadMicros:    200_000,
			CacheWriteMicros:   2_500_000,
			CacheWrite1hMicros: 4_000_000,
		},
		Currency:        "USD",
		SourceURL:       "https://platform.claude.com/docs/en/about-claude/pricing",
		SourceUpdatedAt: "2026-10-26",
	},
	"anthropic/claude-haiku-4-5": {
		Provider: "anthropic", Model: "claude-haiku-4-5",
		Prices: Prices{
			InputMicros:        500_000,
			OutputMicros:       2_500_000,
			CacheReadMicros:    50_000,
			CacheWriteMicros:   625_000,
			CacheWrite1hMicros: 1_000_000,
		},
		Currency:        "USD",
		SourceURL:       "https://platform.claude.com/docs/en/about-claude/pricing",
		SourceUpdatedAt: "2026-10-26",
	},
	"openai/gpt-4o-mini": {
		Provider: "openai", Model: "gpt-4o-mini",
		Prices: Prices{
			InputMicros:     150_000,
			OutputMicros:    600_000,
			CacheReadMicros: 75_000,
		},
		Currency:        "USD",
		SourceURL:       "https://developers.openai.com/api/docs/models/gpt-4o-mini",
		SourceUpdatedAt: "2026-10-26",
	},
}

func priceKey(provider, model string) string {
	return provider + "/" + model
}

// MarketPriceFor returns the bundled public default for a model.
func MarketPriceFor(provider, model string) (MarketPrice, bool) {
	p, ok := marketPrices[priceKey(provider, model)]
	return p, ok
}

// MarketPrices returns every bundled public default in stable order.
func MarketPrices() []MarketPrice {
	keys := slices.Sorted(maps.Keys(marketPrices))
	out := make([]MarketPrice, 0, len(keys))
	for _, key := range keys {
		out = append(out, marketPrices[key])
	}
	return out
}
