package connectors

var identityConnector = Connector{
	ID:       "identity",
	Name:     "Identity actions",
	Category: CategorySecurity,
	Summary:  "Inspect identities and perform narrow account actions such as disabling access or revoking sessions.",
	Maturity: MaturityPlanned,
	Guarantees: []string{
		"subjects are resolved to stable provider identifiers before any action",
		"tenant, group and action type are constrained by connector policy",
		"destructive account actions require a human decision",
	},
	Caveats: []string{
		"does not bypass identity-provider permissions or audit logging",
		"break-glass and offboarding workflows still need explicit policy",
	},
	Operations: []Operation{
		{
			ID:             "identity.read_principal",
			Name:           "Read principal",
			Summary:        "Reads approved user, service account or group metadata by stable identifier.",
			Effects:        []Effect{EffectRead},
			Approval:       ApprovalPolicy,
			SecretHandling: SecretNone,
		},
		{
			ID:             "identity.update_group_membership",
			Name:           "Update group membership",
			Summary:        "Adds or removes a principal from an approved group.",
			Effects:        []Effect{EffectWrite},
			Approval:       ApprovalPolicy,
			SecretHandling: SecretNone,
		},
		{
			ID:             "identity.disable_principal",
			Name:           "Disable principal",
			Summary:        "Disables a user or service account.",
			Effects:        []Effect{EffectDestructive},
			Approval:       ApprovalRequired,
			SecretHandling: SecretNone,
		},
		{
			ID:             "identity.revoke_sessions",
			Name:           "Revoke sessions",
			Summary:        "Revokes active sessions or refresh tokens for an approved principal.",
			Effects:        []Effect{EffectDestructive},
			Approval:       ApprovalRequired,
			SecretHandling: SecretNone,
		},
	},
}
