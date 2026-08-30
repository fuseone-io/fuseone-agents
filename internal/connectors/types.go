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

/*
CachePolicy says whether a governed result may be answered from a cache.

The zero value is undeclared, and resolves to CacheNever. That is the point. A connector shape added
without anyone deciding gets the answer that cannot mislead: a governed read
served from a cache is a record of a read that did not happen, returning data
from an earlier instant under an approval bound to this request.

Deliberately not inferred from the effect. Every read looks cacheable from the
outside, so an effect-derived default would open exactly the operations that
most need the decision made on purpose — the ones that reach a customer system
under an approval.
*/
type CachePolicy string

const (
	// CacheNever answers from the source, always. It is what an undeclared
	// policy resolves to, and it is also spelled out by operations that want
	// the refusal on the record rather than inherited.
	CacheNever CachePolicy = "never"
	// CacheShortLived allows a bounded cache where repeating the call costs
	// more than a slightly older answer, and the operation reaches nothing a
	// person approved for one request.
	CacheShortLived CachePolicy = "short_lived"
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
	// CachePolicy is empty for every operation that has not decided, which
	// reads as CacheNever. Use EffectiveCachePolicy rather than this field.
	CachePolicy CachePolicy
}

// EffectiveCachePolicy is the policy after the zero value is read as never.
// Callers ask this rather than the field, so an undeclared policy cannot be
// mistaken for an absent one.
func (o Operation) EffectiveCachePolicy() CachePolicy {
	if o.CachePolicy == "" {
		return CacheNever
	}
	return o.CachePolicy
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
