package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/gate"
)

/*
One proposal, from the Gate to the far side.

Separate from the loop because they answer different questions. The loop asks
what to do next; this asks whether a particular thing may happen and then makes
it happen — which is where every check, every refusal, and the one irreversible
moment in this platform live.
*/
/*
actApproved makes the call a person cleared.

The arguments come back from the content store rather than from the model,
because what was approved is what the approver read on the screen. Asking for
them again would let a second planning call change them between the agreeing
and the doing.

It still crosses the Gate. The grant answers one check; the budget, the pack
and the policy have all had time to change while the request sat in a queue,
and any of them may now say no.
*/
func (r *Runner) actApproved(ctx context.Context, state State, start Start) (Status, error) {
	approved := state.Approved

	args, err := r.resolve(ctx, approved.ArgsRef)
	if err != nil {
		return Status{}, fmt.Errorf("engine: approved args of %s: %w", start.RunID, err)
	}
	// What the approver saw, byte for byte. A mismatch means the content store
	// answered with something other than what was sealed into the trail, and
	// acting on it would be acting on something nobody approved.
	if got := digest(args); got != approved.ArgsDigest {
		return Status{}, fmt.Errorf(
			"engine: approved args of %s do not match the trail: %s != %s",
			start.RunID, got, approved.ArgsDigest)
	}

	return r.act(ctx, state, start, Proposal{Tool: approved.Tool, Args: args})
}

// resolve reads a stored payload back. An empty reference is an approved call
// that carried no arguments, which is not an error.
func (r *Runner) resolve(ctx context.Context, ref string) ([]byte, error) {
	if ref == "" || r.deps.Content == nil {
		return nil, nil
	}
	return r.deps.Content.Get(ctx, ref)
}

// withThisCall adds the call being ruled on to the planner's estimate.
//
// The planner estimates cost and tokens, which are the things it can guess at.
// How many calls a proposal is, is not a guess: it is one.
func withThisCall(estimate domain.Consumption) domain.Consumption {
	estimate.ToolCalls++
	return estimate
}

func budgetAsConsumption(b domain.Budget) domain.Consumption {
	return domain.Consumption{
		Micros:      b.Micros,
		Tokens:      b.Tokens,
		ToolCalls:   b.ToolCalls,
		Steps:       b.Steps,
		WallClockMS: b.WallClockMS,
	}
}

func budgetEvidence(d domain.Decision) (*domain.Consumption, *domain.Consumption, *domain.Consumption, *domain.Consumption) {
	if d.Breached == "" {
		return nil, nil, nil, nil
	}
	budget := budgetAsConsumption(d.Budget)
	return &budget, &d.Committed, &d.Estimate, &d.Projected
}

// act runs the proposal through the Gate and, if it survives, executes it.
func (r *Runner) act(ctx context.Context, state State, start Start, p Proposal) (Status, error) {
	effect, _ := r.deps.Catalog.Effect(p.Tool)
	idemKey := idempotencyKey(start.RunID, p.Tool, p.Args)

	decision, err := r.deps.Gate.Evaluate(ctx, gate.Request{
		Scope:     start.Scope,
		RunID:     start.RunID,
		AgentID:   start.AgentID,
		Seq:       state.Seq + 1,
		Tool:      p.Tool,
		Effect:    effect,
		Args:      p.Args,
		ArgLabels: state.Labels,
		Pack:      envelopeOf(start, state.Called),
		Stage:     start.Stage,
		Budget:    start.Budget,
		Committed: state.Committed(),
		// The request is itself a call, and the Gate cannot know that on its
		// own. Left out, a ceiling of N tool calls permitted N+1: the check
		// counted the calls already made and never the one it was ruling on.
		Estimate:        withThisCall(p.Estimate),
		IdemKey:         idemKey,
		AlreadyExecuted: state.AlreadyExecuted(idemKey),
		// Only for the exact call that was cleared. A grant that travelled to
		// a different tool, or to the same tool with different arguments,
		// would be the platform doing something nobody agreed to.
		ApprovalGranted: state.Approved != nil &&
			state.Approved.Tool == p.Tool &&
			state.Approved.ArgsDigest == digest(p.Args),
	})
	if err != nil {
		return Status{}, fmt.Errorf("engine: gate: %w", err)
	}

	budget, committed, estimate, projected := budgetEvidence(decision)
	state, err = r.append(ctx, state, start, domain.Step{
		Kind:       domain.StepGateDecided,
		PolicyHash: decision.PolicyHash,
		Payload: mustJSON(domain.GateDecidedPayload{
			Tool: p.Tool, Effect: effect, Verdict: decision.Verdict,
			Rule: decision.Rule, Reason: decision.Reason,
			PolicyCode: decision.PolicyCode, Monitored: decision.Monitored,
			Budget:    budget,
			Committed: committed, Estimate: estimate,
			Projected: projected, Breached: decision.Breached,
			// The inputs beside the outcome, so this decision can be
			// re-evaluated later and not merely replayed (AU-08).
			Labels: state.Labels, ArgsDigest: digest(p.Args),
			Stage: start.Stage,
		}),
	})
	if err != nil {
		return Status{}, err
	}

	if !decision.Verdict.Executable() || decision.Verdict == domain.VerdictRequireApproval {
		return r.refused(ctx, state, start, p, decision, effect)
	}
	return r.invoke(ctx, state, start, p, effect, idemKey)
}

/*
refused records what a call that will not happen does to the run.

Four endings rather than one, because they are four different facts and a run
that reported them the same way would be unreadable: a call waiting for a
person is not a run out of budget, and neither is a planner that has been told
no three times.
*/
func (r *Runner) refused(
	ctx context.Context, state State, start Start,
	p Proposal, decision domain.Decision, effect domain.Effect,
) (Status, error) {
	switch {
	case decision.Verdict == domain.VerdictRequireApproval:
		argsRef, err := r.store(ctx, start.RunID, state.Seq+1, p.Args)
		if err != nil {
			return Status{}, err
		}
		state, err = r.append(ctx, state, start, domain.Step{
			Kind: domain.StepApprovalRequested,
			Payload: mustJSON(domain.ApprovalRequestedPayload{
				Tool: p.Tool, Rule: decision.Rule, Reason: decision.Reason,
				Effect: effect, ArgsRef: argsRef, ArgsDigest: digest(p.Args),
				Estimate: p.Estimate, Labels: state.Labels,
			}),
		})
		return status(state), err

	case decision.Rule == gate.RuleBudget:
		// A budget block parks the run rather than failing it: raising the
		// ceiling resumes from this exact step (PRD FO-04).
		return r.park(ctx, state, start, "budget_exhausted")

	case state.ConsecutiveBlocks >= maxConsecutiveBlocks:
		// The refusal was fed back and the planner kept asking anyway. Waiting
		// for the step ceiling to end this does not work: the Gate reports the
		// most restrictive rule, so a capability refusal masks the budget one,
		// and every further turn is a paid model call that cannot succeed.
		return r.park(ctx, state, start, "no_progress")
	}

	// A first refusal is recorded and fed back so the planner can choose
	// differently. Most refusals cannot be argued with — the pack, the taint
	// and the policy are fixed for the run's version — but a contract refusal
	// is one the model can genuinely fix (PRD SE-09).
	return status(state), nil
}

// park stops the run for a person, with a stable code rather than a sentence.
func (r *Runner) park(ctx context.Context, state State, start Start, reason string) (Status, error) {
	state, err := r.append(ctx, state, start, domain.Step{
		Kind:    domain.StepParked,
		Payload: mustJSON(domain.ParkedPayload{Reason: reason}),
	})
	return status(state), err
}

// invoke reserves budget, calls the tool and reconciles.
func (r *Runner) invoke(
	ctx context.Context, state State, start Start,
	p Proposal, effect domain.Effect, idemKey string,
) (Status, error) {
	call := Call{
		RunID: start.RunID,
		Scope: start.Scope,
		Tool:  p.Tool, Args: p.Args,
		OnBehalfOf: start.OnBehalfOf,
		IdemKey:    idemKey,
	}
	if err := r.deps.Tools.Reserve(ctx, call); err != nil {
		return Status{}, err
	}

	state, err := r.append(ctx, state, start, domain.Step{
		Kind: domain.StepBudgetReserved,
		Payload: mustJSON(domain.BudgetReservedPayload{
			Micros: p.Estimate.Micros, Tokens: p.Estimate.Tokens,
		}),
	})
	if err != nil {
		return Status{}, err
	}

	argsRef, err := r.store(ctx, start.RunID, state.Seq+1, p.Args)
	if err != nil {
		return Status{}, err
	}

	// The idempotency key is recorded with the call, before the effect leaves
	// the process. A crash after this append means the resume sees the key and
	// refuses to call again (PRD DE-16).
	if state, err = r.append(ctx, state, start, domain.Step{
		Kind:    domain.StepToolCalled,
		IdemKey: idemKey,
		Payload: mustJSON(domain.ToolCalledPayload{
			Tool: p.Tool, Effect: effect, ArgsRef: argsRef, ArgsDigest: digest(p.Args),
		}),
	}); err != nil {
		return Status{}, err
	}

	call.Seq = state.Seq
	result, invokeErr := r.deps.Tools.Invoke(ctx, call)
	returned := domain.ToolReturnedPayload{Tool: p.Tool, ResultRef: result.ResultRef}
	if result.Cached {
		returned.Cached = true
		returned.CachedFromRun = result.CachedFromRun
		returned.CachedFromSeq = result.CachedFromSeq
	}
	if result.Failed {
		returned.Failed = true
		returned.ErrorCode = firstNonEmpty(result.ErrorCode, "tool_error")
	}
	if invokeErr != nil {
		returned.Failed = true
		returned.ErrorCode = firstNonEmpty(result.ErrorCode, "invoke_error")
	}

	if state, err = r.append(ctx, state, start, domain.Step{
		Kind:    domain.StepToolReturned,
		Cost:    result.Cost,
		Labels:  result.Labels,
		Payload: mustJSON(returned),
	}); err != nil {
		return Status{}, err
	}

	// Reconcile on every path, including failure. A reservation left
	// outstanding leaks budget for the rest of the run and eventually parks it
	// for no reason.
	state, err = r.append(ctx, state, start, domain.Step{
		Kind: domain.StepBudgetReconciled,
		Payload: mustJSON(domain.BudgetReconciledPayload{
			ReleasedMicros: p.Estimate.Micros, ReleasedTokens: p.Estimate.Tokens,
		}),
	})
	return status(state), err
}

// store puts the proposed arguments in the content store and returns the
// reference the ledger records instead of the bytes.
//
// The sequence is the one the step about to be appended will take, so a
// reference points at the step it belongs to and retention can work per run.
func (r *Runner) store(ctx context.Context, runID domain.RunID, seq int64, args []byte) (string, error) {
	if len(args) == 0 || r.deps.Content == nil {
		return "", nil
	}
	ref, err := r.deps.Content.Put(ctx, runID, seq, args)
	if err != nil {
		return "", fmt.Errorf("engine: store arguments: %w", err)
	}
	return ref, nil
}

// idempotencyKey identifies an effect by what it does, not by where it sits.
//
// The step sequence is deliberately excluded. A resumed run re-plans and lands
// at a different sequence number, so a position-dependent key would look new
// on every retry and duplicate the effect — the exact failure this key exists
// to prevent.
//
// The consequence is that an identical call with identical arguments inside
// one run happens once. For reads that is a free cache hit; for writes a
// second identical call is almost always a bug. A tool that genuinely needs to
// repeat — polling, pagination — varies its arguments, and that is the
// intended escape hatch.
func idempotencyKey(runID domain.RunID, tool domain.ToolID, args []byte) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s|%s|", runID, tool)
	_, _ = h.Write(args)
	return hex.EncodeToString(h.Sum(nil))[:32]
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func digest(b []byte) string {
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:])[:16]
}
