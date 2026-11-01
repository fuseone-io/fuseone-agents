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

/*
Subscriptions is who publishes and who listens, declared here by the consumer
and read from the published specifications.

What a run publishes is what the version it was pinned to declares — the
same version the engine resolved the run from, and the one it judged its
withheld record by. Asking the latest version instead would let a publish in
the middle of a run make the two disagree, either way: a run that emitted
when it started stops emitting, or one that did not starts to. Who listens is
read from the versions published now: a listener chooses to start from an
event when it is published, and a run is opened from the version current
when it opens.
*/
type Subscriptions interface {
	// EmitsOf is what one published version of an agent declares its
	// finished runs publish.
	EmitsOf(ctx context.Context, agent domain.AgentID, version domain.VersionID) ([]domain.AgentEvent, error)
	// Listeners maps an event to the agents it starts.
	Listeners(ctx context.Context) (map[string][]domain.AgentID, error)
}

// Finished lists the runs that have ended well since a moment.
type Finished interface {
	ListRuns(ctx context.Context, filter domain.RunFilter, phase string, limit int) ([]domain.RunSummary, error)
}

// Sources reads, for a batch of finished runs, only each run's opening and
// closing steps: what opened it and its chain, and what it published. One
// read per sweep rather than every step of every run.
type Sources interface {
	StartsAndFinishes(ctx context.Context, runs []domain.RunID) (map[domain.RunID][]domain.Step, error)
}

// Dispatcher opens the runs that finished runs imply.
type Dispatcher struct {
	subs    Subscriptions
	runs    Finished
	sources Sources
	opener  *Opener
	clock   Clock
	log     *slog.Logger
}

// NewDispatcher needs the sources, not only the finished runs: the source
// run's own steps say where it stands in a chain of events (#203), and a
// dispatcher that could not read them would bound nothing.
func NewDispatcher(
	subs Subscriptions, runs Finished, sources Sources, opener *Opener, clock Clock, log *slog.Logger,
) *Dispatcher {
	if log == nil {
		log = slog.Default()
	}
	return &Dispatcher{subs: subs, runs: runs, sources: sources, opener: opener, clock: clock, log: log}
}

// Window is how far back a sweep looks. Generous next to the sweep interval,
// so a pass that failed or a process that restarted still catches what
// finished while it was away.
const Window = 30

// Sweep publishes the events of recently finished runs and returns how many
// runs it opened.
func (d *Dispatcher) Sweep(ctx context.Context, limit int) (int, error) {
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

	pending, err := d.candidates(ctx, finished, listeners)
	if err != nil || len(pending) == 0 {
		return 0, err
	}
	steps, err := d.sources.StartsAndFinishes(ctx, runIDs(pending))
	if err != nil {
		return 0, fmt.Errorf("trigger: read source runs: %w", err)
	}

	opened := 0
	for _, c := range pending {
		src, ok, err := d.sourceOf(c.run, steps[c.run.RunID])
		if err != nil {
			return opened, err
		}
		if !ok {
			continue
		}
		for _, event := range c.events {
			n, err := d.publish(ctx, src, event, listeners[event.Event])
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
		Trigger: domain.TriggerEvent, By: src.run.OnBehalfOf, Input: inputFor(src.run, event, artifacts),
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
