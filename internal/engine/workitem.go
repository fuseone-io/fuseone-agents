package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/fuseone/agents/internal/domain"
)

/*
$fuseone.work_item.claim, executed by the engine itself (#206, NT-013).

The claim is the platform's own record of which run works on which card, and
the ledger is where it lives: the engine appends a work_item_claimed step and
the ledger judges both uniquenesses while it seals it — one card per run, one
open run per card per agent. A refusal comes back as an append error and goes
to the model as a failed result; nothing is sealed for it.

It is handled here rather than in a tool layer because only the run's single
writer may append to its ledger (NF-15), and because no tool server must ever
be able to answer for it: a registered server with a colliding name is never
called, which is part of what keeps the Gate's coordination exemption closed.

It is never answered from the run's own record. Each attempt gets its own
idempotency key and goes to the ledger again, because the answer depends on
every other open run at that moment; the ledger makes a retry of the same card
an idempotent success on its own.
*/

const (
	// CodeWorkItemHeld: another open run of this agent holds the card.
	CodeWorkItemHeld = "work_item_held"
	// CodeWorkItemRunHoldsAnother: this run already holds a different card.
	CodeWorkItemRunHoldsAnother = "work_item_run_holds_another"
	// CodeWorkItemRefInvalid: the arguments name no usable card.
	CodeWorkItemRefInvalid = "work_item_ref_invalid"
)

const coordinationAttemptSeparator = ":attempt:"

type claimArgs struct {
	Ref string `json:"ref"`
}

type claimAnswer struct {
	Claimed bool               `json:"claimed"`
	Ref     domain.WorkItemRef `json:"ref,omitempty"`
	Reason  string             `json:"reason,omitempty"`
	Message string             `json:"message"`
}

// effectOf is the classification the Gate rules on. A coordination tool is a
// write whatever a catalogue says, so it can never be served as a read.
func (r *Runner) effectOf(tool domain.ToolID) domain.Effect {
	if tool.Coordination() {
		return domain.EffectWrite
	}
	effect, _ := r.deps.Catalog.Effect(tool)
	return effect
}

// coordinationIdempotencyKey gives every attempt of a coordination call a key
// of its own, so the ledger records each one and none is mistaken for a
// replay of the last: the first unused of base, base:attempt:2, and so on.
func coordinationIdempotencyKey(state State, base string) string {
	key := base
	for n := 2; state.AlreadyExecuted(key); n++ {
		key = fmt.Sprintf("%s%s%d", base, coordinationAttemptSeparator, n)
	}
	return key
}

// reserve asks the tool layer to get ready, except for a call the engine
// executes itself.
func (r *Runner) reserve(ctx context.Context, call Call) error {
	if call.Tool.Coordination() {
		return nil
	}
	return r.deps.Tools.Reserve(ctx, call)
}

// execute makes the call. A coordination call is the engine's own; anything
// else is fenced on the lease and handed to the tool layer. An invoke error is
// a failed result, not a failed turn: the call was recorded and must return.
func (r *Runner) execute(
	ctx context.Context, state State, start Start, call Call,
) (State, ToolResult, error) {
	if call.Tool == domain.ToolWorkItemClaim {
		return r.claimWorkItem(ctx, state, start, call)
	}
	// The last answer before the effect leaves the process. The call record
	// was fenced, but a worker can be suspended between that write and this
	// line — and renewal only notices at its next tick, after the effect
	// would already be out in the world.
	if start.LeaseEpoch != 0 {
		if err := r.deps.Ledger.Fence(ctx, start.RunID, start.LeaseEpoch, state.Head); err != nil {
			return State{}, ToolResult{}, fmt.Errorf("engine: fence before %s: %w", call.Tool, err)
		}
	}
	result, err := r.deps.Tools.Invoke(ctx, call)
	if err != nil {
		result.Failed = true
		result.ErrorCode = firstNonEmpty(result.ErrorCode, "invoke_error")
	}
	return state, result, nil
}

// claimWorkItem seals the claim, or turns the ledger's refusal into the
// answer the model reads.
func (r *Runner) claimWorkItem(
	ctx context.Context, state State, start Start, call Call,
) (State, ToolResult, error) {
	var args claimArgs
	_ = json.Unmarshal(call.Args, &args)
	ref, ok := domain.ParseWorkItemRef(args.Ref)
	if !ok {
		return r.answerClaim(ctx, state, call, claimAnswer{Reason: CodeWorkItemRefInvalid,
			Message: "ref must be github:owner/repo#123 or ghproject:<node id>"})
	}
	next, err := r.append(ctx, state, start, domain.Step{
		Kind:    domain.StepWorkItemClaimed,
		Labels:  state.Labels.Clone(),
		Payload: mustJSON(domain.WorkItemClaimedPayload{Ref: ref}),
	})
	if err == nil {
		return r.answerClaim(ctx, next, call, claimAnswer{Claimed: true, Ref: ref,
			Message: "the card is this run's until the run ends"})
	}
	refusal, known := claimRefusal(err)
	if !known {
		return State{}, ToolResult{}, err
	}
	refusal.Ref = ref
	return r.answerClaim(ctx, state, call, refusal)
}

// claimRefusal names a ledger refusal for the model. Anything else is not an
// answer about the card and fails the turn.
func claimRefusal(err error) (claimAnswer, bool) {
	switch {
	case errors.Is(err, domain.ErrWorkItemHeld):
		return claimAnswer{Reason: CodeWorkItemHeld,
			Message: "another open run of this agent holds this card; pick another"}, true
	case errors.Is(err, domain.ErrWorkItemRunHoldsAnother):
		return claimAnswer{Reason: CodeWorkItemRunHoldsAnother,
			Message: "this run already holds a different card; a run works on one card"}, true
	case errors.Is(err, domain.ErrWorkItemInvalid):
		return claimAnswer{Reason: CodeWorkItemRefInvalid,
			Message: "the card reference is not usable"}, true
	}
	return claimAnswer{}, false
}

func (r *Runner) answerClaim(
	ctx context.Context, state State, call Call, answer claimAnswer,
) (State, ToolResult, error) {
	body := mustJSON(answer)
	result := ToolResult{
		ResultDigest: ResultDigest(body), ResultBytes: int64(len(body)),
		Failed: !answer.Claimed, ErrorCode: answer.Reason,
	}
	if r.deps.Content != nil {
		ref, err := r.deps.Content.Put(ctx, call.RunID, call.Seq, body)
		if err != nil {
			return State{}, ToolResult{}, fmt.Errorf("engine: store claim result: %w", err)
		}
		result.ResultRef = ref
	}
	return state, result, nil
}
