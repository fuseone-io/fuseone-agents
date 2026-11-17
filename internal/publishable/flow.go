package publishable

import (
	"context"
	"fmt"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/flow"
	"github.com/fuseone/agents/internal/spec"
)

// Rulings is what the Curator has ruled about each tool usable from a scope.
type Rulings map[domain.ToolID]domain.ToolEntry

// Effect is the tool's ruled effect, or unknown when nobody ruled.
func (r Rulings) Effect(tool domain.ToolID) (domain.Effect, bool) {
	entry, ok := r[tool]
	if !ok || !entry.Effect.Valid() {
		return domain.EffectUnknown, false
	}
	return entry.Effect, true
}

// Untrusted is whether the tool's output was ruled untrusted.
func (r Rulings) Untrusted(tool domain.ToolID) bool { return r[tool].Untrusted }

// Rulings is the catalogue with what the Curator decided about each tool,
// narrowed to the tools usable from the agent's scope (NT-019 Enforcement):
// a connector instance's and an MCP server's alike. One out of it is not
// offered, so publishing an agent naming it is refused, and the Gate still
// stops a run that reaches one. Every entry carries its server's runtime
// scope, the installation's by default.
func (c Checker) Rulings(ctx context.Context, scope domain.Scope) (Rulings, error) {
	if c.Tools == nil {
		return Rulings{}, nil
	}
	entries, err := c.Tools.Tools(ctx)
	if err != nil {
		return nil, fmt.Errorf("tool catalogue: %w", err)
	}
	out := make(Rulings, len(entries))
	for _, entry := range entries {
		if !entry.Scope.Contains(scope) {
			continue
		}
		out[entry.ID] = entry
	}
	return out, nil
}

// Envelopes hands the flow checker the steps as data. The flow package cannot
// import the one that parses definitions — dependencies point inward.
func Envelopes(s spec.Spec) []flow.Envelope {
	out := make([]flow.Envelope, 0, len(s.Steps))
	for _, step := range s.Steps {
		out = append(out, flow.Envelope{Name: step.Name, Reaches: step.Reaches})
	}
	return out
}

func blockingStaticFlows(s spec.Spec, catalogue Rulings) []flow.Path {
	finding := flow.Check(s.Tools, Envelopes(s), catalogue)
	var out []flow.Path
	for _, path := range finding.Paths {
		if path.BlocksPublication() {
			out = append(out, path)
		}
	}
	return out
}

func staticFlowBlockDetail(paths []flow.Path) string {
	first := paths[0]
	detail := fmt.Sprintf(
		"untrusted data can reach non-reversible tool %s (%s) from %s",
		first.To, first.Effect, first.From,
	)
	if len(paths) > 1 {
		detail = fmt.Sprintf("%s and %d more path(s)", detail, len(paths)-1)
	}
	return detail
}
