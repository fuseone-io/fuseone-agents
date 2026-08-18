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

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/model"
)

/*
One turn: claim a run, advance it, release it.

The claim is what keeps NF-15 true across replicas — a run belongs to one
worker at a time, and a turn that dies without releasing is picked up by
whoever finds the lease expired, never by two of them at once.
*/
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
			return true, w.park(ctx, claim, advErr, parkedReasonFor(advErr))
		}
		return true, w.release(ctx, claim, outcome)
	}

	log.Debug("advanced", "phase", status.Phase.String(), "seq", status.Seq)
	return true, w.release(ctx, claim, domain.ClaimOutcome{})
}

func parkedReasonFor(err error) string {
	if failure, ok := model.FailureSummaryOf(err); ok {
		return failure.Code
	}
	return "attempts_exhausted"
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
