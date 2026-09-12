package connectors

var graviteeConnector = Connector{
	ID:       "gravitee",
	Name:     "Governed Gravitee subscriptions",
	Category: CategorySecurity,
	Summary:  "Inspect and accept registered Gravitee API subscriptions from a governed ticket without exposing API keys.",
	// Activation is the final slice. A catalogue entry describes the contract;
	// it is not a capability until the transport, reconciliation and end-to-end
	// proofs exist.
	Maturity: MaturityPlanned,
	Guarantees: []string{
		"organization, environment and API references are fixed by connector configuration",
		"acceptance requires the exact ticket revision and inspected subscription snapshot",
		"API keys and connector credentials never enter model-visible results",
	},
	Caveats: []string{
		"the first runtime supports API subscriptions, not API Product subscriptions",
		"an ambiguous acceptance is reconciled and never submitted again automatically",
	},
	Operations: []Operation{
		{
			ID:             "gravitee.inspect_subscription",
			Name:           "Inspect subscription",
			Summary:        "Inspects a pending API subscription into the fixed snapshot shown for approval.",
			Effects:        []Effect{EffectRead},
			Approval:       ApprovalNone,
			SecretHandling: SecretNone,
			CachePolicy:    CacheNever,
		},
		{
			ID:             "gravitee.accept_subscription",
			Name:           "Accept subscription",
			Summary:        "Accepts the inspected API subscription with the approved expiration after a final preflight.",
			Effects:        []Effect{EffectWrite},
			Approval:       ApprovalRequired,
			SecretHandling: SecretNone,
			CachePolicy:    CacheNever,
		},
	},
}
