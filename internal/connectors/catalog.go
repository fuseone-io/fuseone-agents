package connectors

import "slices"

type Category string

const (
	CategoryAutomation     Category = "automation"
	CategoryInfrastructure Category = "infrastructure"
	CategoryMessaging      Category = "messaging"
	CategoryNetwork        Category = "network"
	CategorySecrets        Category = "secrets"
)

type Effect string

const (
	EffectRead        Effect = "read"
	EffectWrite       Effect = "write"
	EffectDestructive Effect = "destructive"
	EffectFinancial   Effect = "financial"
	EffectSecret      Effect = "secret"
)

type Approval string

const (
	ApprovalNone     Approval = "none"
	ApprovalPolicy   Approval = "policy"
	ApprovalRequired Approval = "required"
)

type SecretHandling string

const (
	SecretNone                      SecretHandling = "none"
	SecretReferenceOnly             SecretHandling = "reference_only"
	SecretPlaintextRequiresApproval SecretHandling = "plaintext_requires_approval"
)

type Maturity string

const (
	MaturityPlanned Maturity = "planned"
)

type Operation struct {
	ID             string
	Name           string
	Summary        string
	Effects        []Effect
	Approval       Approval
	SecretHandling SecretHandling
}

type Connector struct {
	ID         string
	Name       string
	Category   Category
	Summary    string
	Maturity   Maturity
	Guarantees []string
	Caveats    []string
	Operations []Operation
}

var catalog = []Connector{
	{
		ID:       "vault",
		Name:     "Vault secret storage",
		Category: CategorySecrets,
		Summary:  "Store generated keys, certificates and operational secrets without returning secret values to the model.",
		Maturity: MaturityPlanned,
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
				Effects:        []Effect{EffectWrite, EffectSecret},
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
				Effects:        []Effect{EffectWrite, EffectSecret},
				Approval:       ApprovalPolicy,
				SecretHandling: SecretReferenceOnly,
			},
		},
	},
	{
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
				ID:             "jobs.run_template",
				Name:           "Run template",
				Summary:        "Starts an approved job template with validated arguments.",
				Effects:        []Effect{EffectWrite, EffectSecret},
				Approval:       ApprovalPolicy,
				SecretHandling: SecretReferenceOnly,
			},
			{
				ID:             "jobs.read_result",
				Name:           "Read result",
				Summary:        "Reads structured job status and non-secret outputs.",
				Effects:        []Effect{EffectRead},
				Approval:       ApprovalNone,
				SecretHandling: SecretNone,
			},
		},
	},
	{
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
	},
	{
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
	},
	{
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
	},
	{
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
				ID:             "http.call_endpoint",
				Name:           "Call endpoint",
				Summary:        "Calls a declared endpoint with schema-checked arguments.",
				Effects:        []Effect{EffectRead, EffectWrite},
				Approval:       ApprovalPolicy,
				SecretHandling: SecretNone,
			},
		},
	},
}

// Catalog returns a defensive copy so callers cannot mutate the shared shape.
func Catalog() []Connector {
	out := make([]Connector, len(catalog))
	for i, c := range catalog {
		out[i] = c
		out[i].Guarantees = slices.Clone(c.Guarantees)
		out[i].Caveats = slices.Clone(c.Caveats)
		out[i].Operations = make([]Operation, len(c.Operations))
		for j, op := range c.Operations {
			out[i].Operations[j] = op
			out[i].Operations[j].Effects = slices.Clone(op.Effects)
		}
	}
	return out
}
