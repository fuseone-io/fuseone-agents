package engine

import (
	"context"
	"fmt"
	"maps"
	"slices"

	"github.com/fuseone/agents/internal/domain"
)

const CodeToolServerUnavailable = "tool_server_unavailable"

// ToolServerUnavailableError is a turn refused before the model was called:
// the run's envelope names tools from a server that is configured and enabled
// but not connected on this worker. Retryable — the worker's reconciler
// retries the connection on a short timer, and backing off is cheaper and
// more honest than planning a run that cannot do its work (#148).
type ToolServerUnavailableError struct {
	Server string
	Tools  []domain.ToolID
}

func (e *ToolServerUnavailableError) Error() string {
	return fmt.Sprintf(
		"engine: tool server %s is configured but not connected here; %d envelope tools cannot be offered",
		e.Server, len(e.Tools))
}

func (e *ToolServerUnavailableError) Summary() domain.FailureSummary {
	return domain.FailureSummary{Code: CodeToolServerUnavailable, Retryable: true}
}

/*
unavailableTools is which of the envelope's tools cannot be offered to the
model this turn, and whether that is an outage.

Resolution mirrors the planner's own: a tool the Schemas port cannot describe
is one the planner would silently skip, and the model cannot call what it is
not offered. Platform-owned tools are never in question — the process itself
serves them.

Two different answers on a miss. A missing tool whose server is configured and
enabled is an outage: the turn refuses to run, retryably, before any money is
spent. A missing tool whose server is gone on purpose is a fact to state — to
the model, so its account of the run is accurate, and on the planned step, so
the trail says what the prompt could not offer.
*/
func (r *Runner) unavailableTools(
	ctx context.Context, offered []domain.ToolID,
) ([]domain.ToolID, error) {
	if r.deps.Schemas == nil {
		return nil, nil
	}
	var missing []domain.ToolID
	for _, id := range offered {
		if id.PlatformOwned() {
			continue
		}
		if _, _, _, ok := r.deps.Schemas.Schema(id); !ok {
			missing = append(missing, id)
		}
	}
	if len(missing) == 0 || r.deps.Servers == nil {
		return missing, nil
	}

	byServer := map[string][]domain.ToolID{}
	for _, id := range missing {
		byServer[id.Server()] = append(byServer[id.Server()], id)
	}
	// Sorted so the server a park names is the same one every attempt.
	for _, server := range slices.Sorted(maps.Keys(byServer)) {
		if server == "" {
			continue
		}
		expected, err := r.deps.Servers.Expected(ctx, server)
		if err != nil {
			return nil, fmt.Errorf("engine: ask whether server %s is expected: %w", server, err)
		}
		if expected {
			return nil, &ToolServerUnavailableError{Server: server, Tools: byServer[server]}
		}
	}
	return missing, nil
}

func toolStrings(ids []domain.ToolID) []string {
	if len(ids) == 0 {
		return nil
	}
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = string(id)
	}
	return out
}
