package connectortools

func sqlRunSchema() map[string]any {
	return map[string]any{
		"template_id": map[string]any{
			"type":        "string",
			"description": "Registered template identifier chosen by the model.",
		},
		"parameters": map[string]any{
			"type":        "object",
			"description": "Values for the registered template's named parameters.",
			"additionalProperties": map[string]any{
				"type": []string{"string", "number", "integer", "boolean"},
			},
		},
	}
}
