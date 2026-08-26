package model

import (
	"testing"

	"github.com/fuseone/agents/internal/domain"
)

func TestProviderOwnedNativeToolSchemasArePropertyMaps(t *testing.T) {
	tests := []struct {
		name   string
		schema map[string]any
		fields []string
	}{
		{
			name:   string(domain.ToolContextRead),
			schema: contextReadToolSchema(),
			fields: []string{"name"},
		},
		{
			name:   string(finishToolID),
			schema: finishToolSchema(),
			fields: []string{"summary", "artifacts", "stopped_by"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requireProviderPropertyMap(t, test.name, test.schema, test.fields)
		})
	}
}

func TestFinishToolSchemaAllowsNestedSchemaKeywords(t *testing.T) {
	schema := finishToolSchema()
	artifacts, ok := schema["artifacts"].(map[string]any)
	if !ok {
		t.Fatalf("artifacts schema = %T, want object schema map", schema["artifacts"])
	}
	if _, ok := artifacts["additionalProperties"]; !ok {
		t.Fatalf("artifacts schema = %+v, want nested additionalProperties", artifacts)
	}
	requireProviderPropertyMap(t, string(finishToolID), schema, []string{"artifacts"})
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
