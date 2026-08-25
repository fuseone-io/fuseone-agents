package connectors

type Category string

const (
	CategoryAutomation     Category = "automation"
	CategoryData           Category = "data"
	CategoryInfrastructure Category = "infrastructure"
	CategoryMessaging      Category = "messaging"
	CategoryNetwork        Category = "network"
	CategorySecrets        Category = "secrets"
	CategorySecurity       Category = "security"
)

type Effect string

const (
	EffectRead        Effect = "read"
	EffectWrite       Effect = "write"
	EffectDestructive Effect = "destructive"
	EffectFinancial   Effect = "financial"
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
	MaturityRuntime Maturity = "runtime"
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

// OperationByID returns the connector and operation for a catalogue operation
// id such as "vault.write_secret".
func OperationByID(id string) (Connector, Operation, bool) {
	for _, connector := range Catalog() {
		for _, operation := range connector.Operations {
			if operation.ID == id {
				return connector, operation, true
			}
		}
	}
	return Connector{}, Operation{}, false
}
