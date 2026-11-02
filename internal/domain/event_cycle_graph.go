package domain

import (
	"cmp"
	"slices"
)

/*
cycleGraph is the event graph as one cycle search reads it: who listens to
each event, which agents can be on a cycle through the one asked about, and
each agent's listeners once the search has reached it.

Every read spends from a budget. Nothing here is proportional to the edges
the declarations would draw — only to the declarations themselves and to the
agents the search actually visits.
*/
type cycleGraph struct {
	wiring    EventWiring
	listeners map[string][]AgentID
	emitters  map[string][]AgentID
	// kept is the agents both reached from the one asked about and reaching
	// it back: the only ones a cycle through it can visit.
	kept map[AgentID]bool
	next map[AgentID][]AgentID

	work, limit int
	exhausted   bool
}

func newCycleGraph(w EventWiring, agent AgentID, limit int) *cycleGraph {
	g := &cycleGraph{
		wiring: w, limit: limit, next: map[AgentID][]AgentID{},
		listeners: map[string][]AgentID{}, emitters: map[string][]AgentID{},
	}
	if !g.index() {
		return g
	}
	reached := g.reach(agent, func(a AgentID) []string { return g.wiring[a].Emits }, g.listeners)
	reaching := g.reach(agent, func(a AgentID) []string { return g.wiring[a].Listens }, g.emitters)
	g.kept = map[AgentID]bool{}
	for a := range reached {
		if reaching[a] {
			g.kept[a] = true
		}
	}
	return g
}

// spend takes one unit of work, or reports the budget gone.
func (g *cycleGraph) spend() bool {
	if g.work >= g.limit {
		g.exhausted = true
		return false
	}
	g.work++
	return true
}

// allow raises the budget, for the part of a search that was kept a reserve.
func (g *cycleGraph) allow(limit int) {
	g.limit, g.exhausted = limit, false
}

// index reads every declaration once, in the agents' name order, so each
// event's listeners and emitters come out sorted.
func (g *cycleGraph) index() bool {
	agents := make([]AgentID, 0, len(g.wiring))
	for a := range g.wiring {
		agents = append(agents, a)
	}
	slices.SortFunc(agents, cmp.Compare[AgentID])
	for _, a := range agents {
		for _, e := range g.wiring[a].Listens {
			if !g.spend() {
				return false
			}
			g.listeners[e] = appendOnce(g.listeners[e], a)
		}
		for _, e := range g.wiring[a].Emits {
			if !g.spend() {
				return false
			}
			g.emitters[e] = appendOnce(g.emitters[e], a)
		}
	}
	return true
}

// appendOnce appends an agent unless it was the last appended: agents come
// in name order, so a repeat is always the last one.
func appendOnce(agents []AgentID, a AgentID) []AgentID {
	if len(agents) > 0 && agents[len(agents)-1] == a {
		return agents
	}
	return append(agents, a)
}

// reach is every agent a walk reaches from the start, through each agent's
// events and each event's agents, reading every event once.
func (g *cycleGraph) reach(start AgentID, events func(AgentID) []string, agentsOf map[string][]AgentID) map[AgentID]bool {
	seen, read := map[AgentID]bool{start: true}, map[string]bool{}
	for queue := []AgentID{start}; len(queue) > 0 && !g.exhausted; queue = queue[1:] {
		for _, e := range events(queue[0]) {
			if read[e] {
				continue
			}
			read[e] = true
			for _, to := range agentsOf[e] {
				if !g.spend() {
					return seen
				}
				if !seen[to] {
					seen[to] = true
					queue = append(queue, to)
				}
			}
		}
	}
	return seen
}

// successors is the kept agents an agent's events start, in name order, read
// once and remembered. False when the budget ran out reading them.
func (g *cycleGraph) successors(a AgentID) ([]AgentID, bool) {
	if g.exhausted {
		return nil, false
	}
	if next, done := g.next[a]; done {
		return next, true
	}
	set := map[AgentID]struct{}{}
	for _, e := range g.wiring[a].Emits {
		for _, to := range g.listeners[e] {
			if !g.spend() {
				return nil, false
			}
			if g.kept[to] {
				set[to] = struct{}{}
			}
		}
	}
	next := make([]AgentID, 0, len(set))
	for to := range set {
		next = append(next, to)
	}
	slices.SortFunc(next, cmp.Compare[AgentID])
	g.next[a] = next
	return next, true
}

// shortestCycle is the shortest way from the agent back to itself, breadth
// first in name order, so the same graph always names the same one. Nil if
// the budget ran out first.
func (g *cycleGraph) shortestCycle(agent AgentID) EventCycle {
	parent := map[AgentID]AgentID{}
	for queue := []AgentID{agent}; len(queue) > 0; queue = queue[1:] {
		next, ok := g.successors(queue[0])
		if !ok {
			return nil
		}
		for _, to := range next {
			if to == agent {
				return pathTo(parent, agent, queue[0])
			}
			if _, seen := parent[to]; !seen {
				parent[to] = queue[0]
				queue = append(queue, to)
			}
		}
	}
	return nil
}

// pathTo walks the parents back from the last agent to the first.
func pathTo(parent map[AgentID]AgentID, first, last AgentID) EventCycle {
	cycle := EventCycle{last}
	for at := last; at != first; {
		at = parent[at]
		cycle = append(cycle, at)
	}
	slices.Reverse(cycle)
	return cycle
}
