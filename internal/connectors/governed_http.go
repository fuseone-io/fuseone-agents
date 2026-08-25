package connectors

var governedHTTPConnector = Connector{
	ID:       "governed-http",
	Name:     "Governed HTTP",
	Category: CategoryNetwork,
	Summary:  "Call tightly-described internal APIs when a dedicated connector does not exist yet.",
	Maturity: MaturityPlanned,
	Guarantees: []string{
		"method, host, path and schema are declared before any call",
		"responses can be compacted or stored by reference before the model sees them",
		"unknown endpoints are unreachable, not merely discouraged",
	},
	Caveats: []string{
		"should graduate into a named connector when the workflow becomes common",
		"does not make an unsafe API safe by itself",
	},
	Operations: []Operation{
		{
			ID:             "governed-http.call_endpoint",
			Name:           "Call endpoint",
			Summary:        "Calls a declared endpoint with schema-checked arguments.",
			Effects:        []Effect{EffectRead, EffectWrite},
			Approval:       ApprovalPolicy,
			SecretHandling: SecretNone,
		},
	},
}
