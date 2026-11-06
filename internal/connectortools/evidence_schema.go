package connectortools

import (
	"fmt"
	"strings"

	"github.com/fuseone/agents/internal/connectors"
)

func evidenceAnalyzeDescription(instance Instance, op connectors.Operation) string {
	var text strings.Builder
	text.WriteString(op.Summary)
	text.WriteString(" Fields kept by each query:")
	for _, query := range instance.Evidence.Queries {
		fmt.Fprintf(&text, " %s: %s.", query.ID, strings.Join(query.Fields, ", "))
	}
	return text.String()
}

// Native schemas return the provider's property map; the wire adds the envelope.
func evidenceCheckSchema() map[string]any {
	return map[string]any{}
}

func evidenceCollectSchema() map[string]any {
	return map[string]any{
		"query": map[string]any{"type": "string",
			"description": "A query this instance declares. Omit when resuming."},
		"params": map[string]any{"type": "object",
			"additionalProperties": map[string]any{"type": "string"},
			"description":          "A value for every parameter of the query."},
		"window": map[string]any{"type": "object",
			"properties": map[string]any{
				"periodSeconds": map[string]any{"type": "integer", "minimum": 1,
					"description": "How far back, for a source with fixed periods; the next accepted one is used."},
				"from": map[string]any{"type": "string", "description": "RFC 3339 start, for a source with absolute ranges."},
				"to":   map[string]any{"type": "string", "description": "RFC 3339 end, for a source with absolute ranges."},
			},
			"additionalProperties": false,
			"description":          "The window. Omit for a lookup with no window, and when resuming."},
		"resume": map[string]any{"type": "string",
			"description": "The evidence reference of a partial collection, to continue it. Nothing else goes with it."},
	}
}

func evidenceAnalyzeSchema() map[string]any {
	fieldList := func(description string) map[string]any {
		return map[string]any{"type": "array", "maxItems": maxEvidenceDimensions,
			"items": map[string]any{"type": "string"}, "description": description}
	}
	return map[string]any{
		"evidence": map[string]any{"type": "string",
			"description": "The evidence reference a collect call in this run returned."},
		"window": map[string]any{"type": "object",
			"properties": map[string]any{
				"from": map[string]any{"type": "string", "description": "RFC 3339 start."},
				"to":   map[string]any{"type": "string", "description": "RFC 3339 end, exclusive."},
			},
			"additionalProperties": false,
			"description":          "Cuts to part of what was collected. It cannot reach outside the collection's window."},
		"groupBy": fieldList("Kept fields to count distinct values of, each on its own."),
		"sum":     fieldList("Kept numeric fields to add up. Values are reported ranked by the first."),
		"match": map[string]any{"type": "object",
			"properties": map[string]any{
				"field":  map[string]any{"type": "string", "description": "A kept field."},
				"equals": map[string]any{"type": "string", "description": "The exact value."},
				"suffix": map[string]any{"type": "string", "description": "A hostname, matching it and its subdomains."},
			},
			"additionalProperties": false,
			"description":          "Counts only matching events. The outcome answers whether any were observed."},
		"top": map[string]any{"type": "integer", "minimum": 1, "maximum": maxEvidenceTop,
			"description": "How many values per group; default 20."},
	}
}
