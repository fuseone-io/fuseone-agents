// Package gate holds the deterministic checks every proposed action crosses
// before it becomes an effect.
//
// The single rule the package exists to enforce: model output is a proposal,
// never an effect. Conversation is free and cheap; action passes through here
// (PRD 10.1).
package gate

import (
	"encoding/json"
	"fmt"

	"github.com/fuseone/agents/internal/domain"
)

/*
The checks themselves.

Each one answers a single question about a single call and returns the same
three shapes, so the order they run in is data rather than control flow and
adding a ninth check is a line in the table, not a branch in a function.
*/
// checkAutonomy asks a person for anything an agent not yet trusted alone
// would do to the world.
func checkAutonomy(r Request) result {
	if r.Stage.NeedsApproval(r.Effect) || (!r.Stage.Valid() && r.Effect != domain.EffectRead) {
		return needsHuman("gate.autonomy.copilot")
	}
	if r.Stage == domain.StageDraft && r.Effect != domain.EffectRead {
		// A draft should never have reached a real run at all — the opener
		// refuses to start one. Reaching here means something opened it
		// anyway, and the answer is still not to act unsupervised.
		return needsHuman("gate.autonomy.draft")
	}
	return pass()
}

func checkCapability(r Request) result {
	if r.Pack.Empty() {
		return stop("the run has no capability pack")
	}
	if r.Pack.Allows(r.Tool) {
		return pass()
	}
	// You may undo what you were allowed to do. The compensating tool is not
	// in the pack because the author never chose it — the Curator decided
	// what undoes what — and the permission it borrows is exactly the one
	// that let the original call happen.
	if r.Compensating != "" && r.Pack.Allows(r.Compensating) {
		return pass()
	}
	return stop(fmt.Sprintf("tool %q is outside the run's capability pack", r.Tool))
}

func checkContract(r Request) result {
	if !r.Effect.Valid() {
		return stop(fmt.Sprintf("tool %q has no effect classification", r.Tool))
	}
	if len(r.Args) > 0 && !json.Valid(r.Args) {
		return stop("arguments are not valid JSON")
	}
	return pass()
}

// checkDataBarrier makes scope labels behave as a data barrier, not as a
// search filter. A run can only act while every company/area label it carries
// is inside the run's own scope. Ordinary approval does not release this:
// cross-company context needs an explicit recorded authorization, not a click
// on a single tool call.
func checkDataBarrier(r Request) result {
	if violation, blocked := r.ArgLabels.ScopeBoundaryViolation(r.Scope); blocked {
		return stop(violation.Error())
	}
	return pass()
}

// checkTaint closes the prompt-injection path: content read from an untrusted
// source at an earlier step must not silently steer an action on the world
// (PRD SE-06).
//
// It escalates by reversibility rather than blocking everything. Nearly every
// useful run reads untrusted input and then writes something — a support agent
// reads the ticket, then leaves a note — so blocking all tainted writes would
// forbid the primary use case while claiming to secure it. A tainted write
// goes to a human; a tainted irreversible action does not happen at all.
//
// The taint here is the run's accumulated context, which is coarse: it marks a
// write as tainted even when the specific arguments came from a trusted
// source. Argument-level provenance is the proper fix and belongs with the
// static data-flow work (PRD F6). Until then the coarse version errs toward
// asking a human, never toward acting unasked.
func checkTaint(r Request) result {
	if !r.ArgLabels.HasAny(domain.LabelUntrusted) {
		return pass()
	}
	switch {
	case r.Effect == domain.EffectRead:
		// A read causes no effect to steer.
		return pass()
	case r.Effect.Reversible():
		return needsHuman("arguments derive from untrusted data")
	default:
		return stop("an irreversible action cannot derive from untrusted data")
	}
}

// checkPolicy is the built-in effect ladder. An installation replaces it with
// its own rules; the ladder is the safe default, not the design.
func checkPolicy(r Request) result {
	// The ladder exists to stop an agent inventing a dangerous call nobody
	// authorised. A compensation is not that: it undoes an act that already
	// crossed this Gate, and it only runs because a person asked for it. Held
	// to the ladder, the effects most worth undoing — financial, destructive —
	// would be exactly the ones that never could be.
	//
	// This lowers the built-in floor and nothing else. An authored rule still
	// decides: mergePolicy takes the stricter of the two, so a deny written
	// against the compensating tool still blocks it.
	if r.Compensating != "" {
		return pass()
	}

	switch r.Effect {
	case domain.EffectRead:
		return pass()
	case domain.EffectWrite:
		return needsHuman("writes require approval by default")
	default:
		return stop("destructive and financial effects are denied by default")
	}
}

func checkBudget(r Request) result {
	projected := domain.Consumption{
		Micros:      r.Committed.Micros + r.Estimate.Micros,
		Tokens:      r.Committed.Tokens + r.Estimate.Tokens,
		ToolCalls:   r.Committed.ToolCalls + r.Estimate.ToolCalls,
		Steps:       r.Committed.Steps + r.Estimate.Steps,
		WallClockMS: r.Committed.WallClockMS + r.Estimate.WallClockMS,
	}
	if dim := r.Budget.Exceeds(projected); dim != "" {
		return result{
			verdict:   domain.VerdictBlock,
			reason:    "would exceed the run's " + dim + " ceiling",
			budget:    r.Budget,
			committed: r.Committed,
			estimate:  r.Estimate,
			projected: projected,
			breached:  dim,
		}
	}
	return pass()
}

func checkIdempotency(r Request) result {
	if r.AlreadyExecuted {
		return duplicate("this effect is already recorded; replaying it would duplicate the effect")
	}
	return pass()
}
