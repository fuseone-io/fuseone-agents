package connectors

var approvedJobsConnector = Connector{
	ID:       "approved-jobs",
	Name:     "Approved automation jobs",
	Category: CategoryAutomation,
	Summary:  "Run pre-approved scripts or workflows, such as CSR generation, without giving the model shell access.",
	Maturity: MaturityPlanned,
	Guarantees: []string{
		"agents choose a registered job template, never an arbitrary command",
		"inputs are schema validated before the job starts",
		"secret outputs are stored as content references for downstream connectors",
	},
	Caveats: []string{
		"job runners still need deployment isolation and source review",
		"templates must declare their outputs before agents can use them",
	},
	Operations: []Operation{
		{
			ID:             "approved-jobs.run_template",
			Name:           "Run template",
			Summary:        "Starts an approved job template with validated arguments.",
			Effects:        []Effect{EffectWrite},
			Approval:       ApprovalPolicy,
			SecretHandling: SecretReferenceOnly,
		},
		{
			ID:             "approved-jobs.read_result",
			Name:           "Read result",
			Summary:        "Reads structured job status and non-secret outputs.",
			Effects:        []Effect{EffectRead},
			Approval:       ApprovalNone,
			SecretHandling: SecretNone,
		},
	},
}
