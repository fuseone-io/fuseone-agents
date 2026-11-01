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
	Listeners(ctx context.Context) (map[string][]domain.EventListener, error)
}

/*
Feed is the finished runs the dispatcher works through, and where it has got
to (#203).

It used to read the thirty newest finished runs on every sweep and remember
nothing: the thirty-first to finish between two sweeps was never dispatched.
Now it reads forward from a durable cursor — in the order the finishing
transactions were allocated their ids, below a boundary where that order is
stable (see domain.FinishCursor) — and moves the cursor only once every run of a page has been dispatched or
refused — so a page that fails is read again, and nothing is left behind.
Two replicas may dispatch the same page; the idempotency key of each opened
run makes the second a no-op, and the cursor keeps the furthest of their
advances.
*/
type Feed interface {
	FinishedSince(ctx context.Context, after domain.FinishCursor, limit int) ([]domain.FinishedRun, error)
	EventCursor(ctx context.Context) (domain.FinishCursor, error)
	AdvanceEventCursor(ctx context.Context, to domain.FinishCursor) error
	// StartsAndFinishes reads, for a batch of finished runs, only each run's
	// opening and closing steps: what opened it and its chain, and what it
	// published. One read per page rather than every step of every run.
	StartsAndFinishes(ctx context.Context, runs []domain.RunSummary) (map[domain.RunID][]domain.Step, error)
}

// Dispatcher opens the runs that finished runs imply.
type Dispatcher struct {
	subs   Subscriptions
	feed   Feed
	opener *Opener
	clock  Clock
	log    *slog.Logger
}

func NewDispatcher(subs Subscriptions, feed Feed, opener *Opener, clock Clock, log *slog.Logger) *Dispatcher {
	if log == nil {
		log = slog.Default()
	}
	return &Dispatcher{subs: subs, feed: feed, opener: opener, clock: clock, log: log}
}

// Page is how many finished runs a sweep reads at a time. A sweep reads page
// after page until it has caught up, so this bounds one read, not how much
// a sweep can dispatch.
const Page = 30

// Sweep publishes the events of the runs that finished since the cursor,
// page after page until it has caught up, and returns how many runs it
// opened. A page that fails leaves the cursor where it was.
func (d *Dispatcher) Sweep(ctx context.Context, limit int) (int, error) {
	listeners, err := d.subs.Listeners(ctx)
	if err != nil {
		return 0, fmt.Errorf("trigger: read listeners: %w", err)
	}
	opened := 0
	for {
		cursor, err := d.feed.EventCursor(ctx)
		if err != nil {
			return opened, fmt.Errorf("trigger: read event cursor: %w", err)
		}
		page, err := d.feed.FinishedSince(ctx, cursor, limit)
		if err != nil || len(page) == 0 {
			return opened, wrapFeed(err)
		}
		n, err := d.dispatchPage(ctx, page, listeners)
		opened += n
		if err != nil {
			return opened, err
		}
		if err := d.feed.AdvanceEventCursor(ctx, page[len(page)-1].At); err != nil {
			return opened, fmt.Errorf("trigger: advance event cursor: %w", err)
		}
		if len(page) < limit {
			return opened, nil
		}
	}
}

func wrapFeed(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("trigger: read finished runs: %w", err)
}

// dispatchPage opens the runs one page of finished runs implies.
func (d *Dispatcher) dispatchPage(
	ctx context.Context, page []domain.FinishedRun, listeners map[string][]domain.EventListener,
) (int, error) {
	pending, err := d.candidates(ctx, page, listeners)
	if err != nil || len(pending) == 0 {
		return 0, err
	}
	steps, err := d.feed.StartsAndFinishes(ctx, summaries(pending))
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
	ctx context.Context, src source, event domain.AgentEvent, listeners []domain.EventListener,
) (int, error) {
	artifacts, err := contextArtifacts(src.run, src.steps, event)
	if err != nil {
		return 0, err
	}
	opened := 0
	for _, listener := range listeners {
		created, err := d.openListener(ctx, src, event, listener, artifacts)
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
	listener domain.EventListener, artifacts []domain.ContextArtifact,
) (bool, error) {
	agent := listener.Agent
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
		// The version that listened when the event went out. A publish that
		// stopped listening since must not start the version that never
		// asked; a superseded listening version still opens, because it is
		// the one that chose to start from this event.
		Version: listener.Version,
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
