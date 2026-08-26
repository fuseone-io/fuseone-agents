package memory

import "testing"

func TestMemoryToolSchemasArePropertyMaps(t *testing.T) {
	tests := map[string]struct {
		schema map[string]any
		fields []string
	}{
		"find": {
			schema: memoryFindSchema(),
			fields: []string{"kind", "subject", "signature", "search", "limit"},
		},
		"suggest": {
			schema: memorySuggestSchema(),
			fields: []string{"kind", "subject", "signature", "claim"},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			for _, key := range []string{"type", "properties", "required", "additionalProperties"} {
				if _, ok := test.schema[key]; ok {
					t.Fatalf("schema includes top-level %q: native schemas must return properties only", key)
				}
			}
			for _, field := range test.fields {
				if _, ok := test.schema[field]; !ok {
					t.Fatalf("schema is missing property %q at the top level", field)
				}
			}
		})
	}
}
