package model

import (
	"fmt"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
)

/*
PriceFor is what this installation pays for one model.

Held by the registry rather than passed by each caller. A run reaches a
provider through the resolver and an authoring call reaches it through the
interview, and a rate threaded through both is a rate that ends up set in one
of them — with the other silently recording zero.

Configured rates are the only rates that feed Cost.Micros. Market defaults are
public reference values, usually in USD, and Cost.Micros is the installation's
currency by domain contract. Mixing those units would make money ceilings fail
open. With no configured rate, zero says "no price in this installation".
*/
func (r *Registry) PriceFor(providerName, modelName string) (Prices, bool, error) {
	r.mu.RLock()
	p, ok := r.providers[providerName]
	r.mu.RUnlock()
	if !ok {
		return Prices{}, false, fmt.Errorf("model: no provider named %q", providerName)
	}
	if price, ok := p.Prices[modelName]; ok {
		return price, true, nil
	}
	return Prices{}, false, nil
}

// withPrice fills a rate the caller did not supply.
//
// Left to the caller it would be supplied in one of the two paths that build a
// planner and forgotten in the other, and the forgotten one records tokens
// with no money against them — silently, and only noticed when somebody asks
// why a month of runs cost nothing.
func (r *Registry) withPrice(providerName string, cfg Config) Config {
	if !cfg.PricePerMTok.IsZero() {
		cfg.PriceConfigured = true
		return cfg
	}
	price, ok, err := r.PriceFor(providerName, cfg.Model)
	if err == nil {
		cfg.PricePerMTok = price
		cfg.PriceConfigured = ok
	}
	// And a way to price a model this planner was not built for. A step may
	// name its own, and without this the planner would bill it at the agent's
	// base rate — the misattribution that made the aggregate worth building
	// per planning call in the first place.
	if cfg.RateFor == nil {
		cfg.RateFor = func(model string) (Prices, bool) {
			price, ok, err := r.PriceFor(providerName, model)
			if err != nil {
				return Prices{}, false
			}
			return price, ok
		}
	}
	return cfg
}

// RateOf reports the rate a planner was built with, for tests and for the
// screens that explain why a figure is zero.
func RateOf(p engine.Planner) Prices {
	switch t := p.(type) {
	case *Anthropic:
		return t.cfg.PricePerMTok
	case *OpenAICompatible:
		return t.cfg.PricePerMTok
	}
	return Prices{}
}

/*
priceUse describes the rate a call was actually billed at.

Takes the resolved rate rather than reading the planner's, so a step that named
its own model reports provenance for that model. Reading cfg here would have
said "configured" about a rate the call did not use.
*/
func priceUse(price Prices, configured bool, cost domain.Cost) domain.ModelPriceUse {
	if !configured {
		return domain.ModelPriceUse{Status: domain.ModelPriceMissing}
	}
	return domain.ModelPriceUse{
		Status:         domain.ModelPriceConfigured,
		NonZeroApplied: price.nonZeroAppliedTo(cost),
	}
}

func (p Prices) nonZeroAppliedTo(cost domain.Cost) bool {
	return cost.InputTokens > 0 && p.InputMicros > 0 ||
		cost.OutputTokens > 0 && p.OutputMicros > 0 ||
		cost.CacheReadTokens > 0 && p.CacheReadMicros > 0 ||
		cost.CacheWriteTokens > 0 && p.CacheWriteMicros > 0
}
