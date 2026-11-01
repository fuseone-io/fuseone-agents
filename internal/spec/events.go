package spec

import (
	"context"
	"fmt"

	"github.com/fuseone/agents/internal/domain"
)

/*
Who publishes what, and what starts whom (PRD SE-10).

Read from the published specifications rather than from a table somebody
maintains beside them. The requirement is that the graph is static and
inspectable, and a graph derived from the definitions cannot disagree with
them: publishing a version that stops emitting an event removes the edge, in
the same act.
*/

// Emitters maps each agent to the events its finished runs publish.
func (r *Registry) Emitters(ctx context.Context) (map[domain.AgentID][]domain.AgentEvent, error) {
	specs, err := r.latest(ctx)
	if err != nil {
		return nil, err
	}
	out := map[domain.AgentID][]domain.AgentEvent{}
	for _, s := range specs {
		if len(s.Emits) > 0 {
			out[s.ID] = []domain.AgentEvent(s.Emits)
		}
	}
	return out, nil
}

// EmitsOf is what one published version declares its finished runs publish.
// Asked of the version a run was pinned to: the latest may say otherwise,
// and the run was resolved — and judged its withheld events — by its own.
func (r *Registry) EmitsOf(ctx context.Context, agent domain.AgentID, version domain.VersionID) ([]domain.AgentEvent, error) {
	s, err := r.Get(ctx, agent, version)
	if err != nil {
		return nil, fmt.Errorf("spec: read %s@%s: %w", agent, version, err)
	}
	return []domain.AgentEvent(s.Emits), nil
}

// Listeners maps each event to the agents it starts, each with the version
// that declared the trigger: the run an event opens is pinned to it, so a
// publish that stops listening in between does not start the version that
// never asked.
func (r *Registry) Listeners(ctx context.Context) (map[string][]domain.EventListener, error) {
	specs, err := r.latest(ctx)
	if err != nil {
		return nil, err
	}
	out := map[string][]domain.EventListener{}
	for _, s := range specs {
		for _, t := range s.Triggers {
			if t.Type == TriggerEvent && t.Event != "" {
				out[t.Event] = append(out[t.Event], domain.EventListener{Agent: s.ID, Version: s.Version})
			}
		}
	}
	return out, nil
}

// Edges is the whole graph, for a screen or a review.
//
// Every edge, including the ones that go nowhere: an event nobody listens to
// and a trigger nothing publishes are the two mistakes this graph exists to
// make visible, and omitting them would leave a picture that looks correct.
func (r *Registry) Edges(ctx context.Context) ([]domain.EventEdge, error) {
	w, err := r.wiring(ctx)
	if err != nil {
		return nil, err
	}
	return EdgesOf(w), nil
}

// latest reads the current version of every published agent.
func (r *Registry) latest(ctx context.Context) ([]Spec, error) {
	summaries, err := r.List(ctx, domain.Scope{}, false)
	if err != nil {
		return nil, fmt.Errorf("spec: list agents: %w", err)
	}

	out := make([]Spec, 0, len(summaries))
	for _, summary := range summaries {
		s, err := r.Get(ctx, summary.ID, summary.VersionID)
		if err != nil {
			return nil, fmt.Errorf("spec: read %s@%s: %w", summary.ID, summary.VersionID, err)
		}
		out = append(out, s)
	}
	return out, nil
}
