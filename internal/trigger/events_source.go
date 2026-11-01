package trigger

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/fuseone/agents/internal/domain"
)

// A finished run as its events see it: what its pinned version declares it
// publishes, and where it stands in a chain of events (#203).

// candidate is a finished run with events somebody listens to.
type candidate struct {
	run    domain.RunSummary
	events []domain.AgentEvent
}

// candidates is the finished runs worth reading: those whose pinned version
// declares an event somebody listens to.
func (d *Dispatcher) candidates(
	ctx context.Context, finished []domain.RunSummary, listeners map[string][]domain.AgentID,
) ([]candidate, error) {
	emits := pinnedEmits{subs: d.subs, seen: map[pinnedVersion][]domain.AgentEvent{}}
	var out []candidate
	for _, run := range finished {
		declared, err := emits.of(ctx, run.AgentID, run.VersionID)
		if err != nil {
			return nil, err
		}
		if events := listened(declared, listeners); len(events) > 0 {
			out = append(out, candidate{run: run, events: events})
		}
	}
	return out, nil
}

func runIDs(pending []candidate) []domain.RunID {
	out := make([]domain.RunID, len(pending))
	for i, c := range pending {
		out[i] = c.run.RunID
	}
	return out
}

type pinnedVersion struct {
	agent   domain.AgentID
	version domain.VersionID
}

// pinnedEmits asks each pinned version once per sweep. A published version
// never changes, so the answer cannot go stale within one.
type pinnedEmits struct {
	subs Subscriptions
	seen map[pinnedVersion][]domain.AgentEvent
}

func (p pinnedEmits) of(
	ctx context.Context, agent domain.AgentID, version domain.VersionID,
) ([]domain.AgentEvent, error) {
	key := pinnedVersion{agent, version}
	if events, ok := p.seen[key]; ok {
		return events, nil
	}
	events, err := p.subs.EmitsOf(ctx, agent, version)
	if err != nil {
		return nil, fmt.Errorf("trigger: events of %s@%s: %w", agent, version, err)
	}
	p.seen[key] = events
	return events, nil
}

// listened is the run's events somebody listens to: nothing else is worth
// reading the run for.
func listened(events []domain.AgentEvent, listeners map[string][]domain.AgentID) []domain.AgentEvent {
	var out []domain.AgentEvent
	for _, event := range events {
		if len(listeners[event.Event]) > 0 {
			out = append(out, event)
		}
	}
	return out
}

// source is a finished run as its events see it: its opening and closing
// steps, and the chain a run its events open would stand in.
type source struct {
	run   domain.RunSummary
	steps []domain.Step
	next  domain.EventChain
}

/*
sourceOf judges a finished run from its opening and closing steps, and
reports false when its events go no further.

A run already MaxEventHops from its root opens nothing (#203), and neither
does one an event opened without a chain. The run itself recorded that on its
closing step, where the trail shows it; here it is only honoured, from the
same recorded start, so a dispatcher on any replica reaches the same answer.
*/
func (d *Dispatcher) sourceOf(run domain.RunSummary, steps []domain.Step) (source, bool, error) {
	position, err := positionOf(run.RunID, steps)
	if err != nil {
		return source{}, false, err
	}
	next, withheld := position.Next(run.RunID)
	if withheld != nil {
		d.log.Info("events withheld", "from", run.RunID, "reason", withheld.Reason,
			"root", withheld.Root, "hop", withheld.Hop, "limit", withheld.Limit)
		return source{}, false, nil
	}
	return source{run: run, steps: steps, next: next}, true, nil
}

// positionOf is the run's place in a chain, from its sealed start. A start
// that will not decode, or is missing, fails the sweep: read as a root, it
// would restart the count.
func positionOf(run domain.RunID, steps []domain.Step) (domain.ChainPosition, error) {
	for _, step := range steps {
		if step.Kind != domain.StepRunStarted {
			continue
		}
		var p domain.RunStartedPayload
		if err := json.Unmarshal(step.Payload, &p); err != nil {
			return domain.ChainPosition{}, fmt.Errorf("trigger: decode start of %s: %w", run, err)
		}
		return domain.ChainPosition{Trigger: p.Trigger, Chain: p.Chain}, nil
	}
	return domain.ChainPosition{}, fmt.Errorf("trigger: source run %s has no start", run)
}
