package trigger

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/fuseone/agents/internal/domain"
)

/*
Composition between agents, by event (PRD SE-10).

One agent declares that it publishes an event; another declares that the event
starts it. Neither names the other. There is no free conversation and no direct
call: an agent that could call another would be a distributed system with no
diagram, and the first incident would be somebody drawing one by hand.

Emission is declared, not chosen. It happens because a run finished and the
specification says this agent publishes that event — a model deciding when to
emit would make the graph a fact about the day rather than about the
definitions.

Dispatch is a sweep rather than a hook on the finishing run. A worker that
crashed between finishing and publishing would drop the event, and a sweep that
runs again cannot: the run it opens carries an idempotency key derived from the
source run, the event and the target, so a second pass reaches the run the
first one already opened.
*/

// Subscriptions is who publishes and who listens, declared here by the
// consumer and read from the published specifications.
type Subscriptions interface {
	// Emitters maps an agent to the events it publishes.
	Emitters(ctx context.Context) (map[domain.AgentID][]domain.AgentEvent, error)
	// Listeners maps an event to the agents it starts.
	Listeners(ctx context.Context) (map[string][]domain.AgentID, error)
}

// Finished lists the runs that have ended well since a moment.
type Finished interface {
	ListRuns(ctx context.Context, filter domain.RunFilter, phase string, limit int) ([]domain.RunSummary, error)
}

// Dispatcher opens the runs that finished runs imply.
type Dispatcher struct {
	subs   Subscriptions
	runs   Finished
	opener *Opener
	clock  Clock
	log    *slog.Logger
}

func NewDispatcher(subs Subscriptions, runs Finished, opener *Opener, clock Clock, log *slog.Logger) *Dispatcher {
	if log == nil {
		log = slog.Default()
	}
	return &Dispatcher{subs: subs, runs: runs, opener: opener, clock: clock, log: log}
}

// Window is how far back a sweep looks. Generous next to the sweep interval,
// so a pass that failed or a process that restarted still catches what
// finished while it was away.
const Window = 30

// Sweep publishes the events of recently finished runs and returns how many
// runs it opened.
func (d *Dispatcher) Sweep(ctx context.Context, limit int) (int, error) {
	emitters, err := d.subs.Emitters(ctx)
	if err != nil {
		return 0, fmt.Errorf("trigger: read emitters: %w", err)
	}
	if len(emitters) == 0 {
		return 0, nil
	}
	listeners, err := d.subs.Listeners(ctx)
	if err != nil {
		return 0, fmt.Errorf("trigger: read listeners: %w", err)
	}
	if len(listeners) == 0 {
		return 0, nil
	}

	finished, err := d.runs.ListRuns(ctx, domain.RunFilter{}, "finished", limit)
	if err != nil {
		return 0, fmt.Errorf("trigger: list finished runs: %w", err)
	}

	opened := 0
	for _, run := range finished {
		for _, event := range emitters[run.AgentID] {
			n, err := d.publish(ctx, run, event, listeners[event.Event])
			if err != nil {
				return opened, err
			}
			opened += n
		}
	}
	return opened, nil
}

// publish opens one run per listener.
func (d *Dispatcher) publish(
	ctx context.Context, source domain.RunSummary, event domain.AgentEvent, listeners []domain.AgentID,
) (int, error) {
	opened := 0
	for _, agent := range listeners {
		// An agent that listens to what it publishes would trigger itself for
		// ever. Refused here rather than at authoring time as well, because a
		// cycle can also be formed by two agents and this is the place that
		// can see the whole graph.
		if agent == source.AgentID {
			continue
		}

		result, err := d.opener.Open(ctx, Request{
			Agent:   agent,
			IdemKey: fmt.Sprintf("event:%s:%s:%s", source.RunID, event.Event, agent),
			Trigger: "event",
			By:      source.OnBehalfOf,
			Input:   inputFor(source, event),
			Labels:  source.Labels.Clone(),
		})
		if err != nil {
			// A listener that is paused, stopped or still a draft is not an
			// error: the platform is doing what somebody configured. Anything
			// else stops the sweep, because it will be the same next time.
			if isRefusal(err) {
				d.log.Debug("event listener did not start",
					"event", event.Event, "agent", agent, "why", err)
				continue
			}
			return opened, fmt.Errorf("trigger: publish %s to %s: %w", event.Event, agent, err)
		}
		if result.Created {
			opened++
			d.log.Info("event opened a run",
				"event", event.Event, "from", source.RunID, "agent", agent, "run", result.RunID)
		}
	}
	return opened, nil
}

// inputFor is what the listening run is about: the event and where it came
// from. Deliberately not the source run's output — an agent that read another
// agent's result would be coupled to its wording, and the trail already links
// the two runs.
func inputFor(source domain.RunSummary, event domain.AgentEvent) []byte {
	input := struct {
		Event     string   `json:"event"`
		FromRun   string   `json:"from_run"`
		FromAgent string   `json:"from_agent"`
		Context   string   `json:"context,omitempty"`
		Artifacts []string `json:"artifacts,omitempty"`
	}{
		Event:     event.Event,
		FromRun:   string(source.RunID),
		FromAgent: string(source.AgentID),
		Context:   event.Context,
		Artifacts: event.Artifacts,
	}
	data, err := json.Marshal(input)
	if err != nil {
		panic(fmt.Sprintf("trigger: encode event input: %v", err))
	}
	return data
}

// isRefusal reports whether the opener declined for a configured reason.
//
// A paused agent, a stopped platform and a draft are all the platform doing
// what somebody asked. They are not failures of the dispatch, and treating
// them as failures would stop every other listener of the same event.
func isRefusal(err error) bool {
	return errors.Is(err, ErrPaused) ||
		errors.Is(err, ErrStopped) ||
		errors.Is(err, ErrDraft) ||
		errors.Is(err, ErrUnknownAgent)
}
