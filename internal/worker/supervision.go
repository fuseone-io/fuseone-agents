// Supervision: what happens to a run that does not advance.
//
// Separate from the loop because the two answer different questions. The loop
// asks what to do next; this asks how many times to try, how long to wait, and
// when to stop and tell somebody — the policy that keeps a failing run from
// burning budget for ever while hiding the fault (PRD NF-14).
package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/model"
)

func (w *Worker) failure(claim domain.Claim, err error) domain.ClaimOutcome {
	attempts := claim.Attempts + 1
	outcome := domain.ClaimOutcome{Err: err}
	if failure, ok := model.FailureSummaryOf(err); ok {
		outcome.Failure = &failure
		if !failure.Retryable {
			outcome.Parked = true
			return outcome
		}
	}
	if attempts >= w.cfg.MaxAttempts {
		outcome.Parked = true
		return outcome
	}
	outcome.NextAttemptAt = w.clock.Now().Add(backoff(w.cfg, attempts))
	return outcome
}

/*
park records in the ledger that the supervisor stopped the run, and why.

Parking decided out here — an unresolvable spec, a planner that never arrived,
attempts exhausted — used to reach only the projection, and the projection is
not the record. A trail that ends without saying the run was parked reads as a
run that stopped mid-turn, and every projection folded from the ledger rather
than from the runs table reports it as still going: the diagram, the simulation
report, replay.

A stable code rather than the error text. The detail is already on the run as
last_error, and a payload the trail renders is a payload somebody reads — one
holding whatever a failure happened to mention is one that eventually carries
personal data through it (AU-04).
*/
func (w *Worker) park(ctx context.Context, claim domain.Claim, reason error, code string) error {
	w.metrics.Park(code)

	head, err := w.deps.Ledger.Head(ctx, claim.RunID)
	if err == nil && head.Kind.Terminal() {
		// The runner already recorded it — a budget ceiling, most often. A
		// second parking would be a correction of something that is not wrong.
		outcome := domain.ClaimOutcome{Err: reason, Parked: true}
		if failure, ok := model.FailureSummaryOf(reason); ok {
			outcome.Failure = &failure
		}
		return w.release(ctx, claim, outcome)
	}

	parked := domain.ParkedPayload{Reason: code, Attempts: claim.Attempts + 1}
	if failure, ok := model.FailureSummaryOf(reason); ok {
		parked.Failure = &failure
	}
	payload, err := json.Marshal(parked)
	if err != nil {
		return fmt.Errorf("encode parking of %s: %w", claim.RunID, err)
	}
	if _, err := w.deps.Ledger.Append(ctx, domain.Step{
		RunID:      claim.RunID,
		Kind:       domain.StepParked,
		Scope:      claim.Scope,
		AgentID:    claim.AgentID,
		VersionID:  claim.VersionID,
		OnBehalfOf: claim.OnBehalfOf,
		At:         w.clock.Now(),
		Payload:    payload,
	}); err != nil {
		// Released anyway: a run left leased because its parking could not be
		// written waits out the whole lease and is then retried by somebody
		// else, which is worse than a projection that knows more than the
		// ledger does.
		w.log.Error("could not record the parking", "run", claim.RunID, "err", err)
	}
	outcome := domain.ClaimOutcome{Err: reason, Parked: true}
	if failure, ok := model.FailureSummaryOf(reason); ok {
		outcome.Failure = &failure
	}
	return w.release(ctx, claim, outcome)
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
