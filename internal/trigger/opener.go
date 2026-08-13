// Package trigger opens runs.
//
// Every way a run can start — a person pressing a button, a schedule coming
// due, a webhook arriving — ends in the same place: one appended step, pinned
// to the version published at that moment, carrying an idempotency key that
// says which intention it belongs to.
//
// One place on purpose. Two paths that both "just append run_started" drift,
// and the way they drift is that one of them forgets the key and starts
// opening a run every time it is retried.
package trigger

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/fuseone/agents/internal/domain"
)

// Ledger is the append-only record, declared here by the consumer.
type Ledger interface {
	Append(ctx context.Context, step domain.Step) (domain.Step, error)
	RunByIdemKey(ctx context.Context, key string) (domain.RunID, error)
}

// Registry resolves an agent to the version published now.
type Registry interface {
	Versions(ctx context.Context, agent domain.AgentID) ([]domain.AgentSummary, error)
}

// Content holds what a run is about. Optional: a run can start with nothing.
type Content interface {
	Put(ctx context.Context, runID domain.RunID, seq int64, data []byte) (string, error)
}

// Clock is the time a run is stamped with.
type Clock interface{ Now() time.Time }

// Stages reports how far an agent is trusted, declared here by the consumer.
//
// Checked here for the same reason a pause is: every way a run can start goes
// through this one place, and a draft that a webhook could start is not a
// draft.
type Stages interface {
	StageOf(ctx context.Context, agent domain.AgentID) (domain.Stage, error)
}

// Pauses reports whether an agent is stopped.
//
// Checked here rather than in each trigger because every way a run can start
// goes through this one place. A pause honoured by the scheduler and not by
// the webhook is a pause that stops an agent on weekdays.
type Pauses interface {
	IsPaused(ctx context.Context, agent domain.AgentID) (bool, error)
}

// Stops are the switches wider than one agent: a scope, or the installation
// (PRD FO-06).
//
// Declared here for the same reason as Pauses, and checked in the same place.
// An incident does not arrive scoped to the agent somebody is looking at, and
// a switch honoured by one trigger and not another is a switch that stops the
// platform on weekdays.
type Stops interface {
	InForce(ctx context.Context) ([]domain.Stop, error)
}

var (
	// ErrPaused means the agent exists and is not running.
	ErrPaused = errors.New("trigger: the agent is paused")
	// ErrStopped means a switch wider than the agent is off (PRD FO-06).
	ErrStopped = errors.New("trigger: the platform is stopped")
	// ErrDraft means the agent has not been let out of Draft. It can be
	// simulated, which is how it earns its way out (PRD FU-10).
	ErrDraft = errors.New("trigger: the agent is still a draft")
)

// Opener turns an intention into a run.
type Opener struct {
	ledger   Ledger
	registry Registry
	content  Content
	pauses   Pauses
	stops    Stops
	stages   Stages
	clock    Clock
}

func NewOpener(ledger Ledger, registry Registry, clock Clock) *Opener {
	return &Opener{ledger: ledger, registry: registry, clock: clock}
}

// WithContent wires where the run's input is stored.
func (o *Opener) WithContent(content Content) *Opener {
	o.content = content
	return o
}

// WithStops wires the switches wider than one agent.
func (o *Opener) WithStops(stops Stops) *Opener {
	o.stops = stops
	return o
}

/*
stopping returns the switch that covers this agent, or nil.

The agent's scope comes from its published version, which means resolving the
registry before the check rather than after. That is the right order anyway: a
stop naming a scope cannot be evaluated against an agent whose scope nobody
looked up, and an agent nobody published is refused by the same lookup.
*/
func (o *Opener) stopping(ctx context.Context, agent domain.AgentID) (*domain.Stop, error) {
	inForce, err := o.stops.InForce(ctx)
	if err != nil {
		return nil, fmt.Errorf("trigger: read stops: %w", err)
	}
	if len(inForce) == 0 {
		return nil, nil
	}

	scope := domain.Scope{}
	// Only worth a lookup when a scope-level switch is on. An installation
	// stop answers without knowing anything about the agent.
	if needsScope(inForce) {
		versions, err := o.registry.Versions(ctx, agent)
		if err != nil {
			return nil, fmt.Errorf("trigger: versions of %s: %w", agent, err)
		}
		if len(versions) > 0 {
			scope = versions[0].Scope
		}
	}

	for _, stop := range inForce {
		if stop.Covers(scope, agent) {
			return &stop, nil
		}
	}
	return nil, nil
}

func needsScope(stops []domain.Stop) bool {
	for _, s := range stops {
		if s.Level == domain.StopScope {
			return true
		}
	}
	return false
}

// WithStages wires how far an agent is trusted, so a draft cannot open a real
// run by any route.
func (o *Opener) WithStages(stages Stages) *Opener {
	o.stages = stages
	return o
}

// WithPauses wires whether an agent is allowed to start at all.
func (o *Opener) WithPauses(pauses Pauses) *Opener {
	o.pauses = pauses
	return o
}
