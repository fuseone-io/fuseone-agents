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
	// The server rides in Provider: the parked payload keeps the summary and
	// drops the error text, and a park that does not name the server sends
	// the operator hunting (#150 review).
	return domain.FailureSummary{
		Code: CodeToolServerUnavailable, Provider: e.Server, Retryable: true,
	}
}

/*
unavailableTools is which of the envelope's tools cannot be offered to the
model this turn, and whether the turn may run at all.

The outage question is answered by connection state, never inferred from
schema presence — the catalogue deliberately keeps a dead session's entries
until the reconciler replaces them, and it hides a live server's tool the
Curator took off the surface. A server the envelope needs that is configured
and enabled but not connected on this worker refuses the turn, retryably,
before any money is spent; every one of its calls would fail.

A tool the Schemas port cannot describe on a server that is connected — or
gone on purpose — is a decision, not an outage. The planner would silently
skip it, and the model cannot call what it is not offered, so the absence is
stated instead: to the model, so its account of the run is accurate, and on
the planned step, so the trail says what the prompt could not offer.
Platform-owned tools are never in question — the process itself serves them.
*/
func (r *Runner) unavailableTools(
	ctx context.Context, offered []domain.ToolID,
) ([]domain.ToolID, error) {
	if r.deps.Schemas == nil {
		return nil, nil
	}
	var missing []domain.ToolID
	servers := map[string][]domain.ToolID{}
	for _, id := range offered {
		if id.PlatformOwned() {
			continue
		}
		if server := id.Server(); server != "" {
			servers[server] = append(servers[server], id)
		}
		if _, _, _, ok := r.deps.Schemas.Schema(id); !ok {
			missing = append(missing, id)
		}
	}
	if r.deps.Connections == nil || r.deps.Servers == nil {
		return missing, nil
	}
	// Sorted so the server a park names is the same one every attempt.
	for _, server := range slices.Sorted(maps.Keys(servers)) {
		if r.deps.Connections.Connected(server) {
			continue
		}
		expected, err := r.deps.Servers.Expected(ctx, server)
		if err != nil {
			return nil, fmt.Errorf("engine: ask whether server %s is expected: %w", server, err)
		}
		if expected {
			return nil, &ToolServerUnavailableError{Server: server, Tools: servers[server]}
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
