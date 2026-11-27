package httpapi

import (
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

func healthFrom(seen domain.IntegrationHealth) openapi.IntegrationHealth {
	out := openapi.IntegrationHealth{
		Reachable: seen.Reachable, ToolCount: seen.ToolCount,
		Detail: ptr(seen.Detail), ObservedAt: seen.ObservedAt,
		ObservedBy: ptr(seen.ObservedBy),
	}
	if seen.LastReachableAt != nil {
		out.LastReachableAt = seen.LastReachableAt
	}
	if seen.ToolCall != nil {
		out.ToolCall = &openapi.IntegrationToolCallHealth{
			Ok:         seen.ToolCall.OK,
			Code:       seen.ToolCall.Code,
			ObservedAt: seen.ToolCall.ObservedAt,
			ObservedBy: ptr(seen.ToolCall.ObservedBy),
			LastOkAt:   seen.ToolCall.LastOKAt,
		}
	}
	return out
}

// awaitingPerson is whether what was seen still says the server waits for a
// person (#483, delivery 3b): unreachable, and observed under the
// configuration as it is now. A binary that predates the wait writes over the
// row without clearing it, and a changed configuration is a new question.
func awaitingPerson(server domain.MCPServer, seen domain.IntegrationHealth) bool {
	return seen.AwaitingPerson && !seen.Reachable && seen.AwaitingGeneration.Equal(server.UpdatedAt)
}
