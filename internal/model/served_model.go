package model

// modelUse resolves the identity and rate of a provider response. Requested
// remains the identity of the call the platform made; Divergence is populated
// only when the provider reports that it served something else.
type modelUse struct {
	Divergence string
	Price      Prices
	Configured bool
}

func resolveModelUse(cfg Config, requested, reported string) modelUse {
	requestedPrice, requestedConfigured := rateForConfig(cfg, requested)
	use := modelUse{Price: requestedPrice, Configured: requestedConfigured}
	if reported == "" || reported == requested {
		return use
	}

	use.Divergence = reported
	// A configured served-model rate is the strongest fact. If the gateway
	// names a model this installation has not priced, retain the requested
	// model's rate instead of turning an already-priced call into zero.
	if cfg.RateFor != nil {
		if servedPrice, ok := cfg.RateFor(reported); ok {
			use.Price, use.Configured = servedPrice, true
		}
	}
	return use
}

func rateForConfig(cfg Config, model string) (Prices, bool) {
	if cfg.RateFor != nil {
		if price, ok := cfg.RateFor(model); ok || model != cfg.Model {
			return price, ok
		}
	}
	return cfg.PricePerMTok, cfg.PriceConfigured
}
