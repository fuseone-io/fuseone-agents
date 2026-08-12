package model

import (
	"fmt"

	"github.com/fuseone/agents/internal/engine"
)

/*
PriceFor is what this installation pays for one model.

Held by the registry rather than passed by each caller. A run reaches a
provider through the resolver and an authoring call reaches it through the
interview, and a rate threaded through both is a rate that ends up set in one
of them — with the other silently recording zero.

Zero is a real answer: it means nobody supplied a rate, so the ledger records
tokens and no money. A default would put a number an operator trusts beside a
figure nobody gave.
*/
func (r *Registry) PriceFor(providerName, modelName string) (Prices, error) {
	r.mu.RLock()
	p, ok := r.providers[providerName]
	r.mu.RUnlock()
	if !ok {
		return Prices{}, fmt.Errorf("model: no provider named %q", providerName)
	}
	return p.Prices[modelName], nil
}

// withPrice fills a rate the caller did not supply.
//
// Left to the caller it would be supplied in one of the two paths that build a
// planner and forgotten in the other, and the forgotten one records tokens
// with no money against them — silently, and only noticed when somebody asks
// why a month of runs cost nothing.
func (r *Registry) withPrice(providerName string, cfg Config) Config {
	if cfg.PricePerMTok != (Prices{}) {
		return cfg
	}
	price, err := r.PriceFor(providerName, cfg.Model)
	if err == nil {
		cfg.PricePerMTok = price
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
