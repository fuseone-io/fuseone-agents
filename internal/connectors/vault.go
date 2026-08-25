package connectors

var vaultConnector = Connector{
	ID:       "vault",
	Name:     "Vault secret storage",
	Category: CategorySecrets,
	Summary:  "Store generated keys, certificates and operational secrets without returning secret values to the model.",
	Maturity: MaturityRuntime,
	Guarantees: []string{
		"secret values are written from content references, not inline model text",
		"reads return metadata or a sealed reference unless a separate policy allows plaintext",
		"paths are constrained by connector policy before a write reaches Vault",
	},
	Caveats: []string{
		"does not generate cryptographic material by itself",
		"does not replace Vault policy; it narrows what the agent can ask for",
	},
	Operations: []Operation{
		{
			ID:             "vault.write_secret",
			Name:           "Write secret",
			Summary:        "Writes a generated key, certificate or secret bundle to an allowed Vault path.",
			Effects:        []Effect{EffectWrite},
			Approval:       ApprovalPolicy,
			SecretHandling: SecretReferenceOnly,
		},
		{
			ID:             "vault.read_metadata",
			Name:           "Read metadata",
			Summary:        "Reads version, lease and path metadata without revealing secret values.",
			Effects:        []Effect{EffectRead},
			Approval:       ApprovalNone,
			SecretHandling: SecretNone,
		},
		{
			ID:             "vault.revoke_lease",
			Name:           "Revoke lease",
			Summary:        "Revokes a Vault lease or generated credential.",
			Effects:        []Effect{EffectWrite},
			Approval:       ApprovalPolicy,
			SecretHandling: SecretReferenceOnly,
		},
	},
}
