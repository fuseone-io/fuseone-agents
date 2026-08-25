package connectortools

func vaultWriteSchema() map[string]any {
	return map[string]any{
		"path": map[string]any{
			"type":        "string",
			"description": "Vault path inside the configured mount and allowed prefixes.",
		},
		"fields": map[string]any{
			"type":        "object",
			"description": "Secret fields to write. Each value names a run context artifact.",
			"additionalProperties": map[string]any{
				"type":     "object",
				"required": []string{"artifact"},
				"properties": map[string]any{
					"artifact": map[string]any{
						"type":        "string",
						"description": "Name of an artifact supplied to this run.",
					},
					"encoding": map[string]any{
						"type":        "string",
						"enum":        []string{"text", "base64"},
						"description": "How to store the artifact bytes. Text is the default.",
					},
				},
				"additionalProperties": false,
			},
		},
	}
}

func vaultReadMetadataSchema() map[string]any {
	return map[string]any{
		"path": map[string]any{
			"type":        "string",
			"description": "Vault path inside the configured mount and allowed prefixes.",
		},
	}
}

func vaultRevokeLeaseSchema() map[string]any {
	return map[string]any{
		"lease_id": map[string]any{
			"type":        "string",
			"description": "Vault lease id to revoke.",
		},
	}
}
