package httpapi

import (
	"context"
	"fmt"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/spec"
)

/*
eventCycleWarning is what a publish says about the cycles the version closes
in the static event graph (#227), or nil when it closes none.

A warning and never a refusal: the factory's Developer ⇄ Reviewer loop is a
cycle somebody designed, and the run-time bound (MaxEventHops) is what stops
one that nobody did. This is the authoring-time half — the author learns of
the loop before the first chain reaches the limit, not from the trail of the
run that did.

Read before the version is written, so a graph that cannot be read refuses a
publish that wrote nothing rather than answering an error for a version that
now exists. The version is judged as it will stand, replacing its agent's
current one. Unwired — a laptop, a test — there is no graph to judge against.
*/
func (s *Server) eventCycleWarning(ctx context.Context, published spec.Spec) (*openapi.EventCycleWarning, error) {
	if s.composition == nil {
		return nil, nil
	}
	edges, err := s.composition.Edges(ctx)
	if err != nil {
		return nil, fmt.Errorf("read the event graph for %s: %w", published.ID, err)
	}
	cycles := domain.EventCyclesThrough(spec.EventGraphWith(edges, published), published.ID)
	if len(cycles) == 0 {
		return nil, nil
	}
	out := &openapi.EventCycleWarning{Cycles: make([][]string, 0, len(cycles)), HopLimit: domain.MaxEventHops}
	for _, cycle := range cycles {
		agents := make([]string, len(cycle))
		for i, agent := range cycle {
			agents[i] = string(agent)
		}
		out.Cycles = append(out.Cycles, agents)
	}
	return out, nil
}
