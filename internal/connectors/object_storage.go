package connectors

var objectStorageConnector = Connector{
	ID:       "object-storage",
	Name:     "Object storage",
	Category: CategoryData,
	Summary:  "Store and retrieve governed artifacts in approved buckets without exposing broad cloud credentials.",
	Maturity: MaturityPlanned,
	Guarantees: []string{
		"bucket and prefix are constrained by connector policy",
		"object bytes move through content references instead of inline model text",
		"delete operations are distinguishable from writes before the provider is called",
	},
	Caveats: []string{
		"does not replace provider IAM, lifecycle or retention policy",
		"does not make objects public unless a future operation declares that effect",
	},
	Operations: []Operation{
		{
			ID:             "object-storage.read_object",
			Name:           "Read object",
			Summary:        "Reads an allowed object into the content store and returns a reference plus metadata.",
			Effects:        []Effect{EffectRead},
			Approval:       ApprovalPolicy,
			SecretHandling: SecretReferenceOnly,
		},
		{
			ID:             "object-storage.write_object",
			Name:           "Write object",
			Summary:        "Writes referenced content to an allowed bucket and prefix.",
			Effects:        []Effect{EffectWrite},
			Approval:       ApprovalPolicy,
			SecretHandling: SecretReferenceOnly,
		},
		{
			ID:             "object-storage.delete_object",
			Name:           "Delete object",
			Summary:        "Deletes an allowed object or version.",
			Effects:        []Effect{EffectDestructive},
			Approval:       ApprovalRequired,
			SecretHandling: SecretNone,
		},
	},
}
