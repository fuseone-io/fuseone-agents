package connectortools

import "testing"

func TestVaultNativeToolSchemasArePropertyMaps(t *testing.T) {
	tests := []struct {
		operation string
		fields    []string
	}{
		{operation: "vault.write_secret", fields: []string{"path", "fields"}},
		{operation: "vault.read_metadata", fields: []string{"path"}},
		{operation: "vault.revoke_lease", fields: []string{"lease_id"}},
	}

	for _, test := range tests {
		t.Run(test.operation, func(t *testing.T) {
			schema, ok := schemaFor(test.operation)
			if !ok {
				t.Fatalf("schemaFor(%q) returned ok=false", test.operation)
			}
			requireProviderPropertyMap(t, test.operation, schema, test.fields)
		})
	}
}

func TestVaultWriteSchemaAllowsNestedSchemaKeywords(t *testing.T) {
	schema := vaultWriteSchema()
	fields, ok := schema["fields"].(map[string]any)
	if !ok {
		t.Fatalf("fields schema = %T, want object schema map", schema["fields"])
	}
	additional, ok := fields["additionalProperties"].(map[string]any)
	if !ok {
		t.Fatalf("fields schema = %+v, want nested additionalProperties", fields)
	}
	if _, ok := additional["required"]; !ok {
		t.Fatalf("nested field schema = %+v, want nested required", additional)
	}
	if _, ok := additional["properties"]; !ok {
		t.Fatalf("nested field schema = %+v, want nested properties", additional)
	}
	requireProviderPropertyMap(t, "vault.write_secret", schema, []string{"path", "fields"})
}

func requireProviderPropertyMap(t *testing.T, name string, schema map[string]any, fields []string) {
	t.Helper()
	for _, key := range []string{"type", "properties", "required", "additionalProperties"} {
		if _, ok := schema[key]; ok {
			t.Fatalf("%s schema includes top-level %q: native schemas must return provider properties only", name, key)
		}
	}
	for _, field := range fields {
		if _, ok := schema[field]; !ok {
			t.Fatalf("%s schema is missing property %q at the top level", name, field)
		}
	}
}
