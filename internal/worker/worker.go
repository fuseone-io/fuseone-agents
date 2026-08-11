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

// Specs resolves everything needed to advance one run of one agent version.
//
// It returns the planner as well as the run configuration because the model,
// the provider and the system prompt are all part of the agent's definition —
// a worker pool serves many agents, and each turn must run under the version
// the run was pinned to.
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
		return true, w.release(ctx, claim, domain.ClaimOutcome{Err: err, Parked: true})
	}

	start := resolved.Start
	start.RunID = claim.RunID
	start.Scope = claim.Scope
	start.AgentID = claim.AgentID
	start.VersionID = claim.VersionID
	start.OnBehalfOf = claim.OnBehalfOf

	// The runner is a thin wrapper over its dependencies, so building one per
	// turn costs nothing and keeps each run on its own agent's planner.
	deps := w.deps
	deps.Planner = resolved.Planner
	status, advErr := engine.NewRunner(deps).Advance(ctx, start)
	if advErr != nil {
		outcome := w.failure(claim, advErr)
		log.Warn("advance failed",
			"attempt", claim.Attempts+1, "parked", outcome.Parked, "err", advErr)
		return true, w.release(ctx, claim, outcome)
	}

	log.Debug("advanced", "phase", status.Phase.String(), "seq", status.Seq)
	return true, w.release(ctx, claim, domain.ClaimOutcome{})
}

// failure decides whether to back off or give up, per the supervision policy.
func (w *Worker) failure(claim domain.Claim, err error) domain.ClaimOutcome {
	attempts := claim.Attempts + 1
	if attempts >= w.cfg.MaxAttempts {
		return domain.ClaimOutcome{Err: err, Parked: true}
	}
	return domain.ClaimOutcome{Err: err, NextAttemptAt: w.clock.Now().Add(backoff(w.cfg, attempts))}
}

func (w *Worker) release(ctx context.Context, claim domain.Claim, outcome domain.ClaimOutcome) error {
	// Release with a fresh context: cancelling the worker must still hand the
	// lease back, or the run waits out the full lease before anyone retries.
	relCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()

	if err := w.queue.Release(relCtx, claim.RunID, outcome); err != nil {
		return fmt.Errorf("release %s: %w", claim.RunID, err)
	}
	return nil
}

// backoff doubles per attempt and caps. Deterministic on purpose: jitter
// belongs at the claim, where many workers contend, not here, where the delay
// is already spread by each run's own failure time.
func backoff(cfg Config, attempts int) time.Duration {
	d := cfg.BaseBackoff
	for range attempts - 1 {
		d *= 2
		if d >= cfg.MaxBackoff {
			return cfg.MaxBackoff
		}
	}
	return d
}

func sleep(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
