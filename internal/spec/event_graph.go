package spec

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"slices"

	"github.com/fuseone/agents/internal/domain"
)

/*
The event graph as a publication judges it (#227): what each agent's current
version declares, not the edges those declarations draw.

The edges are a product — every emitter of an event times every listener of
it — and a judgement made under the graph lock must not pay for it. The cycle
search reads the declarations and the listeners of an event only for the
agents it reaches.
*/

// WiringOf is what these versions declare about events, one entry per agent.
func WiringOf(specs []Spec) domain.EventWiring {
	w := make(domain.EventWiring, len(specs))
	for _, s := range specs {
		w[s.ID] = wiringOf(s)
	}
	return w
}

func wiringOf(s Spec) domain.AgentWiring {
	out := domain.AgentWiring{Listens: listenedEvents(s), Scope: domain.Scope{Company: s.Company, Area: s.Area}}
	for _, e := range s.Emits {
		out.Emits = append(out.Emits, e.Event)
	}
	return out
}

/*
WiringWith is the published wiring as it would stand with this version
current: the version replaces whatever its agent's current one declared, and
every other agent is left as published. A copy, so the caller's map is never
changed under it.
*/
func WiringWith(published domain.EventWiring, draft Spec) domain.EventWiring {
	w := maps.Clone(published)
	if w == nil {
		w = domain.EventWiring{}
	}
	w[draft.ID] = wiringOf(draft)
	return w
}

// listenedEvents is the events whose publication starts this version.
func listenedEvents(s Spec) []string {
	var out []string
	for _, t := range s.Triggers {
		if t.Type == TriggerEvent && t.Event != "" {
			out = append(out, t.Event)
		}
	}
	return out
}

/*
wiring reads every agent's current declarations in one query.

The same choice of current version as List — the one somebody chose, else
the newest — and only the two columns a graph is drawn from. The listing and
a read per agent it replaced cost a round trip per agent, under the lock.
*/
func (r *Registry) wiring(ctx context.Context) (domain.EventWiring, error) {
	rows, err := r.q.Query(ctx, `
		select distinct on (s.agent_id) s.agent_id, s.company_id, s.area_id, s.emits, s.triggers
		from agent_specs s
		left join agent_state st on st.agent_id = s.agent_id
		order by s.agent_id, (st.current_version = s.version_id) desc nulls last,
		         s.published_at desc`)
	if err != nil {
		return nil, fmt.Errorf("spec: read the event wiring: %w", err)
	}
	defer rows.Close()

	w := domain.EventWiring{}
	for rows.Next() {
		var (
			agent, company, area string
			emits, triggers      []byte
		)
		if err := rows.Scan(&agent, &company, &area, &emits, &triggers); err != nil {
			return nil, fmt.Errorf("spec: scan the event wiring: %w", err)
		}
		s, err := declaredEvents(domain.AgentID(agent), emits, triggers)
		if err != nil {
			return nil, err
		}
		s.Company, s.Area = domain.CompanyID(company), domain.AreaID(area)
		w[s.ID] = wiringOf(s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("spec: read the event wiring: %w", err)
	}
	return w, nil
}

// declaredEvents decodes the two columns a graph is drawn from, the same way
// Get decodes them.
func declaredEvents(agent domain.AgentID, emits, triggers []byte) (Spec, error) {
	s := Spec{ID: agent}
	if err := json.Unmarshal(emits, &s.Emits); err != nil {
		return Spec{}, fmt.Errorf("spec: decode emits of %s: %w", agent, err)
	}
	var stored []domain.AgentTrigger
	if err := json.Unmarshal(triggers, &stored); err != nil {
		return Spec{}, fmt.Errorf("spec: decode triggers of %s: %w", agent, err)
	}
	for _, t := range stored {
		s.Triggers = append(s.Triggers, Trigger{Type: t.Type, Event: t.Event})
	}
	return s, nil
}

// EdgesOf is the edges a wiring draws, for a screen: every one, including an
// event nobody listens to and a trigger nothing publishes. In the agents'
// name order, so the same wiring always draws the same list.
func EdgesOf(w domain.EventWiring) []domain.EventEdge {
	agents := slices.SortedFunc(maps.Keys(w), cmp.Compare[domain.AgentID])
	listeners, emitted := map[string][]domain.AgentID{}, map[string]bool{}
	for _, a := range agents {
		for _, e := range w[a].Listens {
			listeners[e] = append(listeners[e], a)
		}
		for _, e := range w[a].Emits {
			emitted[e] = true
		}
	}
	var edges []domain.EventEdge
	for _, from := range agents {
		for _, event := range w[from].Emits {
			edges = append(edges, edgesFrom(from, event, listeners[event])...)
		}
	}
	for _, to := range agents {
		for _, event := range w[to].Listens {
			if !emitted[event] {
				edges = append(edges, domain.EventEdge{Event: event, To: to})
			}
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
