package model

import (
	"encoding/json"

	"github.com/fuseone/agents/internal/domain"
)

const contextReadToolDescription = "Read one context artifact that this event supplied by reference. Use only names listed in the run input."

func contextReadToolSchema() map[string]any {
	return map[string]any{
		"name": map[string]any{
			"type":        "string",
			"description": "The artifact name from this run's context contract.",
		},
	}
}

func isContextReadTool(id domain.ToolID) bool {
	return id == domain.ToolContextRead
}

func contextReadToolSchemaBytes(offered names) int64 {
	total := int64(len(offered.wire[domain.ToolContextRead]) + len(contextReadToolDescription))
	raw, err := json.Marshal(contextReadToolSchema())
	if err == nil {
		total += int64(len(raw))
	}
	return total
}
