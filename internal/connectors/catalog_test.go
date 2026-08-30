package connectors

import (
	"strings"
	"testing"
)

func TestCatalog_hasStableUniqueNamespacedOperations(t *testing.T) {
	t.Parallel()

	connectors := Catalog()
	seenConnectors := map[string]bool{}
	seenOperations := map[string]bool{}
	for _, connector := range connectors {
		if connector.ID == "" {
			t.Fatal("connector with empty id")
		}
		if seenConnectors[connector.ID] {
			t.Fatalf("duplicate connector id %q", connector.ID)
		}
		seenConnectors[connector.ID] = true
		for _, op := range connector.Operations {
			if !strings.HasPrefix(op.ID, connector.ID+".") {
				t.Fatalf("%s operation %q is not namespaced by the connector id", connector.ID, op.ID)
			}
			if seenOperations[op.ID] {
				t.Fatalf("duplicate operation id %q", op.ID)
			}
			seenOperations[op.ID] = true
		}
	}
}

func TestCatalog_usesOnlyDeclaredEnumValues(t *testing.T) {
	t.Parallel()

	for _, connector := range Catalog() {
		if !validCategories[connector.Category] {
			t.Fatalf("%s category = %q", connector.ID, connector.Category)
		}
		if !validMaturities[connector.Maturity] {
			t.Fatalf("%s maturity = %q", connector.ID, connector.Maturity)
		}
		for _, op := range connector.Operations {
			if !validApprovals[op.Approval] {
				t.Fatalf("%s approval = %q", op.ID, op.Approval)
			}
			if !validSecretHandling[op.SecretHandling] {
				t.Fatalf("%s secret handling = %q", op.ID, op.SecretHandling)
			}
			for _, effect := range op.Effects {
				if !validEffects[effect] {
					t.Fatalf("%s effect = %q", op.ID, effect)
				}
			}
		}
	}
}

func TestCatalog_secretHandlingIsNotDuplicatedAsAnEffect(t *testing.T) {
	t.Parallel()

	for _, connector := range Catalog() {
		for _, op := range connector.Operations {
			for _, effect := range op.Effects {
				if string(effect) == "secret" {
					t.Fatalf("%s duplicates secret handling as an effect", op.ID)
				}
			}
		}
	}
}

func TestCatalog_containsConnectorPriorities(t *testing.T) {
	t.Parallel()

	seen := map[string]bool{}
	for _, connector := range Catalog() {
		seen[connector.ID] = true
	}
	for _, id := range []string{"sql", "object-storage", "identity"} {
		if !seen[id] {
			t.Fatalf("missing governed connector priority %q", id)
		}
	}
}

func TestCatalog_sqlIsReadOnlyAndTemplateBased(t *testing.T) {
	t.Parallel()

	connector := connectorByID(t, "sql")
	if connector.Maturity != MaturityRuntime {
		t.Fatalf("sql maturity = %q, want the proved runtime", connector.Maturity)
	}
	if !hasGuarantee(connector, "queries run from registered read-only templates, not arbitrary SQL text") {
		t.Fatal("sql connector does not declare template-only reads")
	}
	for _, op := range connector.Operations {
		for _, effect := range op.Effects {
			if effect != EffectRead {
				t.Fatalf("%s effect = %q, want read-only", op.ID, effect)
			}
		}
	}
	if len(connector.Operations) != 1 || connector.Operations[0].ID != "sql.run_query_template" {
		t.Fatalf("sql operations = %+v, want only the implemented template read", connector.Operations)
	}
}

func TestCatalog_objectStorageMovesBytesByReference(t *testing.T) {
	t.Parallel()

	connector := connectorByID(t, "object-storage")
	if !hasGuarantee(connector, "object bytes move through content references instead of inline model text") {
		t.Fatal("object storage does not declare reference-based content movement")
	}
	for _, id := range []string{"object-storage.read_object", "object-storage.write_object"} {
		op := operationByID(t, connector, id)
		if op.SecretHandling != SecretNone {
			t.Fatalf("%s secret handling = %q, want none", id, op.SecretHandling)
		}
	}
}

func TestCatalog_nonReversibleEffectsRequireApproval(t *testing.T) {
	t.Parallel()

	for _, connector := range Catalog() {
		for _, op := range connector.Operations {
			if !hasNonReversibleEffect(op) {
				continue
			}
			if op.Approval != ApprovalRequired {
				t.Fatalf("%s has a non-reversible effect with approval %q, want required", op.ID, op.Approval)
			}
		}
	}
}

func TestCatalog_returnsADeepCopy(t *testing.T) {
	t.Parallel()

	first := Catalog()
	first[0].Guarantees[0] = "mutated guarantee"
	first[0].Caveats[0] = "mutated caveat"
	first[0].Operations[0].Summary = "mutated operation"
	first[0].Operations[0].Effects[0] = EffectFinancial

	again := Catalog()
	if again[0].Guarantees[0] == "mutated guarantee" {
		t.Fatal("guarantees share backing storage")
	}
	if again[0].Caveats[0] == "mutated caveat" {
		t.Fatal("caveats share backing storage")
	}
	if again[0].Operations[0].Summary == "mutated operation" {
		t.Fatal("operations share backing storage")
	}
	if again[0].Operations[0].Effects[0] == EffectFinancial {
		t.Fatal("operation effects share backing storage")
	}
}

func connectorByID(t *testing.T, id string) Connector {
	t.Helper()
	for _, connector := range Catalog() {
		if connector.ID == id {
			return connector
		}
	}
	t.Fatalf("missing connector %q", id)
	return Connector{}
}

func operationByID(t *testing.T, connector Connector, id string) Operation {
	t.Helper()
	for _, op := range connector.Operations {
		if op.ID == id {
			return op
		}
	}
	t.Fatalf("missing operation %q", id)
	return Operation{}
}

func hasEffect(op Operation, effect Effect) bool {
	for _, candidate := range op.Effects {
		if candidate == effect {
			return true
		}
	}
	return false
}

func hasNonReversibleEffect(op Operation) bool {
	return hasEffect(op, EffectDestructive) || hasEffect(op, EffectFinancial)
}

func hasGuarantee(connector Connector, guarantee string) bool {
	for _, candidate := range connector.Guarantees {
		if candidate == guarantee {
			return true
		}
	}
	return false
}

var validCategories = map[Category]bool{
	CategoryAutomation:     true,
	CategoryData:           true,
	CategoryInfrastructure: true,
	CategoryMessaging:      true,
	CategoryNetwork:        true,
	CategorySecrets:        true,
	CategorySecurity:       true,
}

var validEffects = map[Effect]bool{
	EffectRead:        true,
	EffectWrite:       true,
	EffectDestructive: true,
	EffectFinancial:   true,
}

var validApprovals = map[Approval]bool{
	ApprovalNone:     true,
	ApprovalPolicy:   true,
	ApprovalRequired: true,
}

var validSecretHandling = map[SecretHandling]bool{
	SecretNone:                      true,
	SecretReferenceOnly:             true,
	SecretPlaintextRequiresApproval: true,
}

var validMaturities = map[Maturity]bool{
	MaturityPlanned: true,
	MaturityRuntime: true,
}

/*
Every operation says whether its result may be cached, and silence means no.

The zero value is `never` on purpose. A connector shape added without thinking
about caching gets the answer that cannot leak: a governed read served from a
cache is a record of a read that did not happen, against data from an earlier
instant. Inferring the policy from the effect would give the opposite default,
because every read looks cacheable from the outside.
*/
func TestCatalog_everyOperationHasACachePolicyAndSilenceMeansNever(t *testing.T) {
	t.Parallel()

	for _, connector := range Catalog() {
		for _, op := range connector.Operations {
			switch op.EffectiveCachePolicy() {
			case CacheNever, CacheShortLived:
			default:
				t.Errorf("%s: cache policy %q is not a declared value",
					op.ID, op.EffectiveCachePolicy())
			}
			if op.CachePolicy == "" && op.EffectiveCachePolicy() != CacheNever {
				t.Errorf("%s: an undeclared policy must read as never", op.ID)
			}
		}
	}
}

// SQL reaches a customer database under an approval bound to one request. A
// second run answered from a cache would report a read that never happened, so
// the refusal is written down rather than left to the default.
func TestCatalog_sqlNeverServesAResultFromCache(t *testing.T) {
	t.Parallel()

	for _, connector := range Catalog() {
		if connector.ID != "sql" {
			continue
		}
		for _, op := range connector.Operations {
			if op.CachePolicy != CacheNever {
				t.Errorf("%s: cache policy = %q, want an explicit never", op.ID, op.CachePolicy)
			}
		}
	}
}
