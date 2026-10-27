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

The outage question is answered by connection state and ownership, never
inferred from schema presence or from an ID's prefix. A tool the MCP
catalogue owns whose server holds no live session is heading for a certain
invoke failure; a tool the catalogue does not own — a native connector's,
whatever its name looks like — has no MCP session to be missing. Only a
tool that is both undescribable and owned by nobody falls back to its
prefix: absence has no owner, and the prefix plus configuration is the one
signal left — it is what keeps a server whose entries were dropped on
disconnect parking its runs while the reconciler fails to bring it back.

A server implicated either way that is configured and enabled refuses the
turn, retryably, before any money is spent. Every other undescribable tool
is a decision, not an outage: the absence is stated to the model, so its
account of the run is accurate, and on the planned step, so the trail says
what the prompt could not offer. Platform-owned tools are never in question.
*/
func (r *Runner) unavailableTools(
	ctx context.Context, offered []domain.ToolID,
) ([]domain.ToolID, error) {
	if r.deps.Schemas == nil {
		return nil, nil
	}
	var missing []domain.ToolID
	implicated := map[string][]domain.ToolID{}
	for _, id := range offered {
		if id.PlatformOwned() {
			continue
		}
		_, _, _, described := r.deps.Schemas.Schema(id)
		if !described {
			missing = append(missing, id)
		}
		if r.deps.Connections == nil {
			continue
		}
		server, live, owned := r.deps.Connections.Session(id)
		switch {
		case owned && !live:
			implicated[server] = append(implicated[server], id)
		case !owned && !described:
			// Only a dead prefix implicates: a live server that stopped
			// offering the tool after a re-list made a decision, and an
			// outage claim about it would park the run for ever.
			if prefix := id.Server(); prefix != "" && !r.deps.Connections.Connected(prefix) {
				implicated[prefix] = append(implicated[prefix], id)
			}
		}
	}
	if r.deps.Connections == nil || r.deps.Servers == nil {
		return missing, nil
	}
	// Sorted so the server a park names is the same one every attempt.
	for _, server := range slices.Sorted(maps.Keys(implicated)) {
		expected, err := r.deps.Servers.Expected(ctx, server)
		if err != nil {
			return nil, fmt.Errorf("engine: ask whether server %s is expected: %w", server, err)
		}
		if expected {
			return nil, &ToolServerUnavailableError{Server: server, Tools: implicated[server]}
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
