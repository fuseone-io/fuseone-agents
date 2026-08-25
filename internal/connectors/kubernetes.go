package connectors

var kubernetesConnector = Connector{
	ID:       "kubernetes",
	Name:     "Kubernetes operations",
	Category: CategoryInfrastructure,
	Summary:  "Inspect workloads and perform narrowly-approved operational actions in Kubernetes clusters.",
	Maturity: MaturityPlanned,
	Guarantees: []string{
		"cluster, namespace and verb are part of connector policy",
		"read operations are shaped before they enter the prompt",
		"write operations carry explicit effects for Gate review",
	},
	Caveats: []string{
		"does not grant broad kubeconfig access to the model",
		"destructive actions require tighter policies than read diagnostics",
	},
	Operations: []Operation{
		{
			ID:             "kubernetes.describe_workload",
			Name:           "Describe workload",
			Summary:        "Reads deployment, pod and event state for allowed namespaces.",
			Effects:        []Effect{EffectRead},
			Approval:       ApprovalNone,
			SecretHandling: SecretNone,
		},
		{
			ID:             "kubernetes.restart_workload",
			Name:           "Restart workload",
			Summary:        "Restarts a named workload inside an allowed namespace.",
			Effects:        []Effect{EffectWrite},
			Approval:       ApprovalPolicy,
			SecretHandling: SecretNone,
		},
	},
}
