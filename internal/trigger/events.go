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

type RunReader interface {
	Read(ctx context.Context, runID domain.RunID, fromSeq int64) ([]domain.Step, error)
}

// Dispatcher opens the runs that finished runs imply.
type Dispatcher struct {
	subs   Subscriptions
	runs   Finished
	reader RunReader
	opener *Opener
	clock  Clock
	log    *slog.Logger
}

// NewDispatcher needs the reader, not only the finished runs: the source
// run's own steps say where it stands in a chain of events (#203), and a
// dispatcher that could not read them would bound nothing.
func NewDispatcher(
	subs Subscriptions, runs Finished, reader RunReader, opener *Opener, clock Clock, log *slog.Logger,
) *Dispatcher {
	if log == nil {
		log = slog.Default()
	}
	return &Dispatcher{subs: subs, runs: runs, reader: reader, opener: opener, clock: clock, log: log}
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
		events := listened(emitters[run.AgentID], listeners)
		if len(events) == 0 {
			continue
		}
		src, ok, err := d.readSource(ctx, run)
		if err != nil {
			return opened, err
		}
		if !ok {
			continue
		}
		for _, event := range events {
			n, err := d.publish(ctx, src, event, listeners[event.Event])
			if err != nil {
				return opened, err
			}
			opened += n
		}
	}
	return opened, nil
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

// source is a finished run as its events see it: its own steps, and the
// chain a run its events open would stand in.
type source struct {
	run   domain.RunSummary
	steps []domain.Step
	next  domain.EventChain
}

/*
readSource reads the finished run once, for its context and its place in a
chain, and reports false when its events go no further.

A run already MaxEventHops from its root opens nothing (#203). The run itself
recorded that on its closing step, where the trail shows it; here it is only
honoured, from the same recorded start, so a dispatcher on any replica
reaches the same answer.
*/
func (d *Dispatcher) readSource(ctx context.Context, run domain.RunSummary) (source, bool, error) {
	steps, err := d.reader.Read(ctx, run.RunID, domain.FirstSeq)
	if err != nil {
		return source{}, false, fmt.Errorf("trigger: read source run %s: %w", run.RunID, err)
	}
	chain, err := chainOf(run.RunID, steps)
	if err != nil {
		return source{}, false, err
	}
	next, ok := chain.Next(run.RunID)
	if !ok {
		d.log.Info("events withheld: the chain reached its hop limit",
			"from", run.RunID, "root", chain.Root, "hop", chain.Hop, "limit", domain.MaxEventHops)
		return source{}, false, nil
	}
	return source{run: run, steps: steps, next: next}, true, nil
}

// chainOf is the chain sealed on the run's start. A start that will not
// decode fails the sweep: read as a root, it would restart the count.
func chainOf(run domain.RunID, steps []domain.Step) (*domain.EventChain, error) {
	for _, step := range steps {
		if step.Kind != domain.StepRunStarted {
			continue
		}
		var p domain.RunStartedPayload
		if err := json.Unmarshal(step.Payload, &p); err != nil {
			return nil, fmt.Errorf("trigger: decode start of %s: %w", run, err)
		}
		return p.Chain, nil
	}
	return nil, fmt.Errorf("trigger: source run %s has no start", run)
}

// publish opens one run per listener.
func (d *Dispatcher) publish(
	ctx context.Context, src source, event domain.AgentEvent, listeners []domain.AgentID,
) (int, error) {
	artifacts, err := contextArtifacts(src.run, src.steps, event)
	if err != nil {
		return 0, err
	}
	opened := 0
	for _, agent := range listeners {
		created, err := d.openListener(ctx, src, event, agent, artifacts)
		if err != nil {
			return opened, err
		}
		if created {
			opened++
		}
	}
	return opened, nil
}

func (d *Dispatcher) openListener(
	ctx context.Context, src source, event domain.AgentEvent,
	agent domain.AgentID, artifacts []domain.ContextArtifact,
) (bool, error) {
	// An agent that listens to what it publishes would trigger itself for
	// ever. Refused at once rather than after MaxEventHops runs: a cycle of
	// one is never what anybody meant.
	if agent == src.run.AgentID {
		return false, nil
	}
	next := src.next
	result, err := d.opener.Open(ctx, Request{
		Agent: agent, IdemKey: fmt.Sprintf("event:%s:%s:%s", src.run.RunID, event.Event, agent),
		Trigger: "event", By: src.run.OnBehalfOf, Input: inputFor(src.run, event, artifacts),
		Labels: src.run.Labels.Clone(), ContextArtifacts: artifacts, Chain: &next,
	})
	if err != nil {
		if isRefusal(err) {
			d.log.Debug("event listener did not start", "event", event.Event, "agent", agent, "why", err)
			return false, nil
		}
		return false, fmt.Errorf("trigger: publish %s to %s: %w", event.Event, agent, err)
	}
	if result.Created {
		d.log.Info("event opened a run",
			"event", event.Event, "from", src.run.RunID, "agent", agent, "run", result.RunID)
	}
	return result.Created, nil
}

// inputFor is what the listening run is about: the event and where it came
// from. Deliberately not the source run's output — an agent that read another
// agent's result would be coupled to its wording, and the trail already links
// the two runs.
func inputFor(source domain.RunSummary, event domain.AgentEvent, artifacts []domain.ContextArtifact) []byte {
	input := struct {
		Event            string                   `json:"event"`
		FromRun          string                   `json:"from_run"`
		FromAgent        string                   `json:"from_agent"`
		Context          string                   `json:"context,omitempty"`
		Artifacts        []string                 `json:"artifacts,omitempty"`
		ContextArtifacts []domain.ContextArtifact `json:"context_artifacts,omitempty"`
	}{
		Event:            event.Event,
		FromRun:          string(source.RunID),
		FromAgent:        string(source.AgentID),
		Context:          event.Context,
		Artifacts:        event.Artifacts,
		ContextArtifacts: artifacts,
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
	var boundary domain.ScopeBoundaryViolation
	return errors.Is(err, ErrPaused) ||
		errors.Is(err, ErrStopped) ||
		errors.Is(err, ErrDraft) ||
		errors.Is(err, ErrUnknownAgent) ||
		errors.As(err, &boundary)
}
