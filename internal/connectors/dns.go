package connectors

var dnsConnector = Connector{
	ID:       "dns",
	Name:     "DNS management",
	Category: CategoryNetwork,
	Summary:  "Read and change DNS records within approved zones.",
	Maturity: MaturityPlanned,
	Guarantees: []string{
		"zones and record types are constrained before the provider is called",
		"changes are recorded as write effects",
		"destructive record removal is distinguishable from an upsert",
	},
	Caveats: []string{
		"does not bypass provider-side IAM",
		"propagation and cache behaviour remain outside the platform",
	},
	Operations: []Operation{
		{
			ID:             "dns.read_record",
			Name:           "Read record",
			Summary:        "Reads records from an allowed DNS zone.",
			Effects:        []Effect{EffectRead},
			Approval:       ApprovalNone,
			SecretHandling: SecretNone,
		},
		{
			ID:             "dns.upsert_record",
			Name:           "Upsert record",
			Summary:        "Creates or updates a record inside an allowed DNS zone.",
			Effects:        []Effect{EffectWrite},
			Approval:       ApprovalPolicy,
			SecretHandling: SecretNone,
		},
		{
			ID:             "dns.delete_record",
			Name:           "Delete record",
			Summary:        "Deletes a record inside an allowed DNS zone.",
			Effects:        []Effect{EffectDestructive},
			Approval:       ApprovalRequired,
			SecretHandling: SecretNone,
		},
	},
}
