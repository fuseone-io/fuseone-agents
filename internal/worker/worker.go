// Package worker turns the agent loop into continuous operation.
//
// A worker claims a run, advances it by one turn, and releases it. Nothing is
// held between turns: the ledger carries the state, so any worker can pick up
// any run, and a worker that dies mid-turn costs at most one lease expiry
// rather than a stuck run (PRD NF-02, NF-13).
package worker

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/fuseone/agents/internal/compensate"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
)

// Queue is the coordination surface a worker needs. Declared here, by the
// consumer; the ledger implementations satisfy it structurally, so no store
// ever imports this package.
type Queue interface {
	// Claim leases one claimable run to owner, or returns
	// domain.ErrNoClaimableRun.
	Claim(ctx context.Context, owner string, lease time.Duration) (domain.Claim, error)
	// Release ends a lease. Progress resets the failure count; a failure
	// schedules the next attempt and records why.
	Release(ctx context.Context, runID domain.RunID, outcome domain.ClaimOutcome) error
}

// Stages is how far each agent is trusted to act alone, declared here by the
// consumer. Optional: a pool without it treats every agent as a draft, which
// escalates every effect and is the safe reading of a missing wire.
type Stages interface {
	StageOf(ctx context.Context, agent domain.AgentID) (domain.Stage, error)
}

// Specs resolves everything needed to advance one run of one agent version.
//
// It returns the planner as well as the run configuration because the model,
// the provider and the system prompt are all part of the agent's definition —
// a worker pool serves many agents, and each turn must run under the version
// the run was pinned to.
// Ceilings is what a scope may spend, declared here by the consumer.
//
// Optional: an installation that has configured none runs on the ceilings in
// each agent's specification alone.
type Ceilings interface {
	Resolve(ctx context.Context, scope domain.Scope) (domain.Budget, domain.Period, error)
	SpentSince(ctx context.Context, scope domain.Scope, since time.Time) (domain.Consumption, error)
}

type Specs interface {
	Resolve(ctx context.Context, agent domain.AgentID, version domain.VersionID) (Resolution, error)
}

// errNoPlanner means a spec resolved but carries nothing to plan with.
var errNoPlanner = errors.New("worker: the resolved agent version has no planner")

// Resolution is one agent version, ready to run.
type Resolution struct {
	Start   engine.Start
	Planner engine.Planner
}

// Config tunes one worker pool.
type Config struct {
	// Owner identifies this process in a lease. A pod name is ideal: it makes
	// an expired lease traceable to the machine that dropped it.
	Owner string
	// Concurrency is how many runs this pool advances at once. It is the
	// resource limit that matters, and it is per pool so an agent that needs
	// isolation gets its own deployment (PRD DE-01 notes on Kubernetes).
	Concurrency int
	// Lease must outlast the slowest single turn, or a second worker will pick
	// up a run that is still being advanced. Idempotency keys make that safe
	// rather than catastrophic, but it is still wasted work.
	Lease time.Duration
	// IdleWait is how long to pause when the queue is empty.
	IdleWait time.Duration
	// MaxAttempts is how many consecutive failures a run takes before it is
	// parked for a human. Never unbounded: a run that fails for ever burns
	// budget and hides the fault (PRD NF-14).
	MaxAttempts int
	// BaseBackoff is doubled per attempt, capped at MaxBackoff.
	BaseBackoff time.Duration
	MaxBackoff  time.Duration
}

func (c *Config) withDefaults() {
	if c.Concurrency <= 0 {
		c.Concurrency = 4
	}
	if c.Lease <= 0 {
		c.Lease = 2 * time.Minute
	}
	if c.IdleWait <= 0 {
		c.IdleWait = time.Second
	}
	if c.MaxAttempts <= 0 {
		c.MaxAttempts = 5
	}
	if c.BaseBackoff <= 0 {
		c.BaseBackoff = 2 * time.Second
	}
	if c.MaxBackoff <= 0 {
		c.MaxBackoff = 5 * time.Minute
	}
}

// Worker is one pool.
type Worker struct {
	cfg   Config
	queue Queue
	// deps carries every collaborator except the planner, which is resolved
	// per run from the agent's own definition.
	deps  engine.Deps
	specs Specs
	clock engine.Clock
	log   *slog.Logger

	// ceilings is optional; nil means the only budget is the agent's own.
	ceilings Ceilings
	// stages is optional; nil means every agent is treated as a draft.
	stages Stages
	// undos is what takes each tool back. Optional: an installation that has
	// ruled on nothing can still abandon a run, and every act comes back
	// reported as standing rather than quietly dropped.
	undos compensate.Catalogue
}

// WithUndos wires the Curator's ruling on what compensates what.
func (w *Worker) WithUndos(c compensate.Catalogue) *Worker {
	w.undos = c
	return w
}

// WithStages wires how far each agent is trusted.
func (w *Worker) WithStages(s Stages) *Worker {
	w.stages = s
	return w
}

// WithCeilings wires the scope budgets a run is narrowed by.
func (w *Worker) WithCeilings(c Ceilings) *Worker {
	w.ceilings = c
	return w
}

func New(cfg Config, queue Queue, deps engine.Deps, specs Specs, clock engine.Clock, log *slog.Logger) *Worker {
	cfg.withDefaults()
	if log == nil {
		log = slog.Default()
	}
	return &Worker{cfg: cfg, queue: queue, deps: deps, specs: specs, clock: clock, log: log}
}

// Run advances claimed runs until ctx is cancelled.
func (w *Worker) Run(ctx context.Context) error {
	var wg sync.WaitGroup
	wg.Add(w.cfg.Concurrency)

	for slot := range w.cfg.Concurrency {
		go func() {
			defer wg.Done()
			w.loop(ctx, slot)
		}()
	}

	wg.Wait()
	return ctx.Err()
}

func (w *Worker) loop(ctx context.Context, slot int) {
	log := w.log.With("owner", w.cfg.Owner, "slot", slot)

	for {
		if err := ctx.Err(); err != nil {
			return
		}

		worked, err := w.turn(ctx, log)
		switch {
		case err != nil && ctx.Err() == nil:
			log.Error("turn failed outside a run", "err", err)
		case worked:
			// Straight back for the next claim: a busy queue should not pay
			// the idle wait between every run.
			continue
		}

		if !sleep(ctx, w.cfg.IdleWait) {
			return
		}
	}
}
