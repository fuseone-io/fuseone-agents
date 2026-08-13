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
func (r *Registry) Emitters(ctx context.Context) (map[domain.AgentID][]string, error) {
	specs, err := r.latest(ctx)
	if err != nil {
		return nil, err
	}
	out := map[domain.AgentID][]string{}
	for _, s := range specs {
		if len(s.Emits) > 0 {
			out[s.ID] = s.Emits
		}
	}
	return out, nil
}

// Listeners maps each event to the agents it starts.
func (r *Registry) Listeners(ctx context.Context) (map[string][]domain.AgentID, error) {
	specs, err := r.latest(ctx)
	if err != nil {
		return nil, err
	}
	out := map[string][]domain.AgentID{}
	for _, s := range specs {
		for _, t := range s.Triggers {
			if t.Type == "event" && t.Event != "" {
				out[t.Event] = append(out[t.Event], s.ID)
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
	emitters, err := r.Emitters(ctx)
	if err != nil {
		return nil, err
	}
	listeners, err := r.Listeners(ctx)
	if err != nil {
		return nil, err
	}

	var edges []domain.EventEdge
	published := map[string]bool{}
	for agent, events := range emitters {
		for _, event := range events {
			published[event] = true
			if len(listeners[event]) == 0 {
				edges = append(edges, domain.EventEdge{From: agent, Event: event})
				continue
			}
			for _, to := range listeners[event] {
				edges = append(edges, domain.EventEdge{From: agent, Event: event, To: to})
			}
		}
	}
	for event, agents := range listeners {
		if published[event] {
			continue
		}
		for _, to := range agents {
			edges = append(edges, domain.EventEdge{Event: event, To: to})
		}
	}
	return edges, nil
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
