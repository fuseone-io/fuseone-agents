package connectors

var smtpConnector = Connector{
	ID:       "smtp",
	Name:     "Outbound email",
	Category: CategoryMessaging,
	Summary:  "Send governed notifications and templated emails without turning email into a chat channel.",
	Maturity: MaturityPlanned,
	Guarantees: []string{
		"recipients and sender identities are constrained by policy",
		"templates keep model text out of headers",
		"external delivery is recorded as a write effect",
	},
	Caveats: []string{
		"does not receive human replies; use channels for conversations",
		"mail provider bounce semantics remain provider-specific",
	},
	Operations: []Operation{
		{
			ID:             "smtp.send_template",
			Name:           "Send template",
			Summary:        "Sends an approved template to allowed recipients.",
			Effects:        []Effect{EffectWrite},
			Approval:       ApprovalPolicy,
			SecretHandling: SecretNone,
		},
	},
}
