package connectors

var sqlConnector = Connector{
	ID:       "sql",
	Name:     "Governed SQL read access",
	Category: CategoryData,
	Summary:  "Read approved database views through registered templates, without giving the model arbitrary SQL execution.",
	Maturity: MaturityPlanned,
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
			ID:             "sql.describe_schema",
			Name:           "Describe schema",
			Summary:        "Reads approved table, view and column metadata without returning rows.",
			Effects:        []Effect{EffectRead},
			Approval:       ApprovalPolicy,
			SecretHandling: SecretNone,
		},
		{
			ID:             "sql.run_query_template",
			Name:           "Run query template",
			Summary:        "Runs a registered read-only query template with validated parameters and row limits.",
			Effects:        []Effect{EffectRead},
			Approval:       ApprovalPolicy,
			SecretHandling: SecretNone,
		},
		{
			ID:             "sql.lookup_row",
			Name:           "Lookup row",
			Summary:        "Reads a bounded record or small row set from an approved view by stable keys.",
			Effects:        []Effect{EffectRead},
			Approval:       ApprovalPolicy,
			SecretHandling: SecretNone,
		},
	},
}
