package connectors

var sqlConnector = Connector{
	ID:       "sql",
	Name:     "Governed SQL read access",
	Category: CategoryData,
	Summary:  "Read approved database views through registered templates, without giving the model arbitrary SQL execution.",
	Maturity: MaturityRuntime,
	Guarantees: []string{
		"queries run from registered read-only templates, not arbitrary SQL text",
		"tables, columns, filters and row limits are declared before execution",
		"large result sets can be stored or compacted before the model sees them",
	},
	Caveats: []string{
		"does not replace database permissions or row-level security",
		"does not support writes; use a dedicated connector for business actions",
	},
	Operations: []Operation{
		{
			ID:             "sql.run_query_template",
			Name:           "Run query template",
			Summary:        "Runs a registered read-only query template with validated parameters and row limits.",
			Effects:        []Effect{EffectRead},
			Approval:       ApprovalPolicy,
			SecretHandling: SecretNone,
			// Written down rather than inherited. A governed read reaches a
			// customer database under an approval bound to one request, so an
			// answer from a cache would report a read that did not happen.
			CachePolicy: CacheNever,
		},
	},
}
