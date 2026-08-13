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
	"fmt"
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

// turn claims one run, advances it once and releases it. The bool reports
// whether any work was found.
func (w *Worker) turn(ctx context.Context, log *slog.Logger) (bool, error) {
	claim, err := w.queue.Claim(ctx, w.cfg.Owner, w.cfg.Lease)
	switch {
	case errors.Is(err, domain.ErrNoClaimableRun):
		return false, nil
	case err != nil:
		return false, fmt.Errorf("claim: %w", err)
	}

	log = log.With("run", claim.RunID, "agent", claim.AgentID)

	resolved, err := w.specs.Resolve(ctx, claim.AgentID, claim.VersionID)
	if err == nil && resolved.Planner == nil {
		// A resolution with no planner is a misconfiguration, not a transient
		// fault. Handing the nil to the runner panics mid-run and leaves the
		// lease to expire, so the next worker repeats it.
		err = fmt.Errorf("%w: %s@%s", errNoPlanner, claim.AgentID, claim.VersionID)
	}
	if err != nil {
		// A run whose spec cannot be resolved will not fix itself by retrying:
		// park it so someone sees it.
		return true, w.park(ctx, claim, err, "spec_unresolved")
	}

	// The agent's own ceiling is per run; the scope's is over a window. They
	// combine by narrowing, so whichever binds first stops the run — and it
	// parks resumably either way, which is what makes raising a ceiling resume
	// from the exact step rather than repeat an effect (PRD FO-02, FO-04).
	budget, err := w.scopedBudget(ctx, claim.Scope, resolved.Start.Budget)
	if err != nil {
		return true, w.park(ctx, claim, err, "budget_unreadable")
	}

	start := resolved.Start
	start.Budget = budget
	start.RunID = claim.RunID
	start.Scope = claim.Scope
	start.AgentID = claim.AgentID
	start.VersionID = claim.VersionID
	start.OnBehalfOf = claim.OnBehalfOf

	// Read per claim rather than cached: promotion has to take effect on the
	// next turn, and a demotion has to take effect faster than that.
	if start.Stage, err = w.stageOf(ctx, claim.AgentID); err != nil {
		return true, w.park(ctx, claim, err, "stage_unreadable")
	}

	// Not every claimable run wants advancing. One a person abandoned wants
	// undoing, and asking the model what to do next would be asking it to
	// carry on with a run somebody already ended.
	if claim.Phase == engine.PhaseCompensating.String() {
		return true, w.undo(ctx, claim, start, log)
	}

	// The runner is a thin wrapper over its dependencies, so building one per
	// turn costs nothing and keeps each run on its own agent's planner.
	deps := w.deps
	deps.Planner = resolved.Planner
	status, advErr := engine.NewRunner(deps).Advance(ctx, start)
	if advErr != nil {
		outcome := w.failure(claim, advErr)
		log.Warn("advance failed",
			"attempt", claim.Attempts+1, "parked", outcome.Parked, "err", advErr)
		if outcome.Parked {
			return true, w.park(ctx, claim, advErr, "attempts_exhausted")
		}
		return true, w.release(ctx, claim, outcome)
	}

	log.Debug("advanced", "phase", status.Phase.String(), "seq", status.Seq)
	return true, w.release(ctx, claim, domain.ClaimOutcome{})
}

// scopedBudget narrows a run's ceiling by what its scope has left.
//
// Read per claim rather than cached: a ceiling raised because a run parked has
// to take effect on the next attempt, which is the whole point of parking
// resumably rather than failing.
// stageOf reads how far this agent is trusted.
//
// A failure parks the run rather than guessing. Guessing high would let an
// untrusted agent act alone because a query failed, and guessing low would
// send every action of a trusted one to a person who did not ask for them.
func (w *Worker) stageOf(ctx context.Context, agent domain.AgentID) (domain.Stage, error) {
	if w.stages == nil {
		return domain.StageDraft, nil
	}
	stage, err := w.stages.StageOf(ctx, agent)
	if err != nil {
		return "", fmt.Errorf("read the stage of %s: %w", agent, err)
	}
	return stage, nil
}

func (w *Worker) scopedBudget(ctx context.Context, scope domain.Scope, agent domain.Budget) (domain.Budget, error) {
	if w.ceilings == nil {
		return agent, nil
	}

	ceiling, period, err := w.ceilings.Resolve(ctx, scope)
	if err != nil {
		// Refusing to run is the safe failure: proceeding would spend against
		// a ceiling nobody could read.
		return domain.Budget{}, fmt.Errorf("resolve budget for %s: %w", scope, err)
	}
	if period == "" {
		return agent, nil
	}

	spent, err := w.ceilings.SpentSince(ctx, scope, period.Since(w.clock.Now()))
	if err != nil {
		return domain.Budget{}, fmt.Errorf("read spend for %s: %w", scope, err)
	}
	return agent.Narrow(domain.Headroom(ceiling, spent)), nil
}
