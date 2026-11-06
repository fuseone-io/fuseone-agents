package connectors

var evidenceConnector = Connector{
	ID:       "evidence",
	Name:     "Governed evidence collection",
	Category: CategorySecurity,
	Summary:  "Collect events from a declared source and analyze them in aggregate, with the coverage of every number stated.",
	Maturity: MaturityRuntime,
	Guarantees: []string{
		"queries are named declarations; a model supplies values that match a declared pattern, never query text",
		"every count carries its window, scope and coverage; a number that hit a limit is reported as a floor",
		"a refusal from the source is a failure, never zero events",
		"events stay in the content store under a run's retention; a model receives a manifest and aggregates",
	},
	Caveats: []string{
		"read-only is the credential's guarantee; declare a read-only token",
		"it reads what the source records; an event the source does not keep cannot be observed",
	},
	Operations: []Operation{
		{
			ID:             "evidence.check",
			Name:           "Check source",
			Summary:        "Validates the credential and runs the source's positive control.",
			Effects:        []Effect{EffectRead},
			Approval:       ApprovalNone,
			SecretHandling: SecretNone,
			CachePolicy:    CacheNever,
		},
		{
			ID:             "evidence.collect",
			Name:           "Collect evidence",
			Summary:        "Runs one named query over a window and returns a reference to the events with their coverage.",
			Effects:        []Effect{EffectRead},
			Approval:       ApprovalPolicy,
			SecretHandling: SecretNone,
			CachePolicy:    CacheNever,
		},
		{
			ID:             "evidence.analyze",
			Name:           "Analyze evidence",
			Summary:        "Aggregates events this run collected, carrying their coverage into the answer.",
			Effects:        []Effect{EffectRead},
			Approval:       ApprovalPolicy,
			SecretHandling: SecretNone,
			CachePolicy:    CacheNever,
		},
	},
}
