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
	specs, err := r.latest(ctx)
	if err != nil {
		return nil, err
	}
	return EdgesOf(specs), nil
}

// EdgesOf is the graph the given current versions draw, as Edges reads it:
// pure, so a fake publisher judges cycles by the same graph the real one does.
func EdgesOf(specs []Spec) []domain.EventEdge {
	emitters := map[string][]domain.AgentID{}
	listeners := map[string][]domain.AgentID{}
	for _, s := range specs {
		for _, declared := range s.Emits {
			emitters[declared.Event] = append(emitters[declared.Event], s.ID)
		}
		for _, event := range listenedEvents(s) {
			listeners[event] = append(listeners[event], s.ID)
		}
	}

	var edges []domain.EventEdge
	for event, froms := range emitters {
		for _, from := range froms {
			edges = append(edges, edgesFrom(from, event, listeners[event])...)
		}
	}
	for event, tos := range listeners {
		if len(emitters[event]) > 0 {
			continue
		}
		for _, to := range tos {
			edges = append(edges, domain.EventEdge{Event: event, To: to})
		}
	}
	return edges
}

// edgesFrom is one emitter's edges for one event: one per listener, or one
// that goes nowhere when nobody listens.
func edgesFrom(from domain.AgentID, event string, listeners []domain.AgentID) []domain.EventEdge {
	if len(listeners) == 0 {
		return []domain.EventEdge{{From: from, Event: event}}
	}
	out := make([]domain.EventEdge, 0, len(listeners))
	for _, to := range listeners {
		out = append(out, domain.EventEdge{From: from, Event: event, To: to})
	}
	return out
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
