package model

// objectSchema builds the object a function's parameters are sent as, keeping
// the fields its author marked as required.
func objectSchema(schema map[string]any) map[string]any {
	fields, required := propertiesOf(schema)
	out := map[string]any{"type": "object", "properties": fields}
	if len(required) > 0 {
		out["required"] = required
	}
	return out
}

/*
propertiesOf reads a tool's fields and the ones it requires, however its
schema was written.

This provider is given the fields and builds the object around them. A schema
written as the whole object — which is what the other provider's wire format
takes — would arrive as a property called "type" and one called "properties",
and the request is refused for a schema that is invalid with no word about
which tool wrote it. Both shapes exist in this repository, so both are read
here rather than at each author. The required list travels with the fields:
dropped, every field becomes optional and the model proposes calls the
connector then refuses.
*/
func propertiesOf(schema map[string]any) (map[string]any, []string) {
	if schema["type"] != "object" {
		return schema, nil
	}
	fields, ok := schema["properties"].(map[string]any)
	if !ok {
		return schema, nil
	}
	return fields, requiredOf(schema["required"])
}

// requiredOf reads the list as Go writes it or as JSON decodes it.
func requiredOf(v any) []string {
	switch list := v.(type) {
	case []string:
		return list
	case []any:
		out := make([]string, 0, len(list))
		for _, item := range list {
			if name, ok := item.(string); ok {
				out = append(out, name)
			}
		}
		return out
	}
	return nil
}
