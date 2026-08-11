// Package gate holds the deterministic checks every proposed action crosses
// before it becomes an effect.
//
// The single rule the package exists to enforce: model output is a proposal,
// never an effect. Conversation is free and cheap; action passes through here
// (PRD 10.1).
package gate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/fuseone/agents/internal/domain"
)

// Rule names identify which check produced a decision. The trail never reads
// "denied by policy" — it names the rule (PRD AU-10).
const (
	// RulePassed is the rule of an action no check objected to. It exists so
	// an allow never borrows the name of a check that had nothing to say: the
	// trail renders rule beside verdict, and "allow / capability" reads as a
	// contradiction the operator has to decipher.
	RulePassed      = "passed"
	RuleCapability  = "capability"
	RuleContract    = "contract"
	RuleTaint       = "taint"
	RulePolicy      = "policy"
	RuleBudget      = "budget"
	RuleIdempotency = "idempotency"
	RuleApproval    = "approval"
)

// policyVersion must be bumped whenever the semantics of a built-in check
// change, even when the rule names stay the same.
const policyVersion = "builtin/v1"

// result is a check's answer: a verdict plus why.
type result struct {
	verdict domain.Verdict
	reason  string
}

func pass() result                 { return result{verdict: domain.VerdictAllow} }
func stop(why string) result       { return result{domain.VerdictBlock, why} }
func needsHuman(why string) result { return result{domain.VerdictRequireApproval, why} }

type check struct {
	rule string
	eval func(Request) result
}

// checkOrder is normative, not incidental. Cheap absolute checks run before
// expensive contextual ones, and the earliest block is the one reported: it is
// the failure the operator actually has to fix.
var checkOrder = []check{
	{RuleCapability, checkCapability},
	{RuleContract, checkContract},
	{RuleTaint, checkTaint},
	{RulePolicy, checkPolicy},
	{RuleBudget, checkBudget},
	{RuleIdempotency, checkIdempotency},
}

// Gate evaluates requests against a fixed, versioned rule set.
type Gate struct {
	policyHash string
}

func New() *Gate {
	return &Gate{policyHash: builtinPolicyHash()}
}

// Evaluate runs every check in order and returns the most restrictive ruling.
//
// A block short-circuits: nothing after it can loosen the outcome. Softer
// rulings are remembered and evaluation continues, so a later block still
// wins over an earlier approval requirement.
func (g *Gate) Evaluate(_ context.Context, r Request) (domain.Decision, error) {
	worst := domain.Decision{
		Verdict:    domain.VerdictAllow,
		Rule:       RulePassed,
		PolicyHash: g.policyHash,
	}

	for _, c := range checkOrder {
		got := c.eval(r)
		if got.verdict == domain.VerdictAllow {
			continue
		}
		d := domain.Decision{
			Verdict:    got.verdict,
			Rule:       c.rule,
			Reason:     got.reason,
			PolicyHash: g.policyHash,
		}
		if got.verdict == domain.VerdictBlock {
			return d, nil
		}
		if got.verdict > worst.Verdict {
			worst = d
		}
	}

	// A human clearance releases an action that only ever needed approval. It
	// is applied last on purpose: a grant must never override a block raised
	// by any check above.
	if worst.Verdict == domain.VerdictRequireApproval && r.ApprovalGranted {
		return domain.Decision{
			Verdict:    domain.VerdictAllow,
			Rule:       RuleApproval,
			Reason:     "approved by a human for this run",
			PolicyHash: g.policyHash,
		}, nil
	}
	return worst, nil
}

// builtinPolicyHash pins the rule set that produced a decision. Changing any
// rule changes the hash, which is what lets a replay tell "the policy said
// yes" apart from "a different policy said yes" (PRD AU-08).
func builtinPolicyHash() string {
	h := sha256.New()
	for _, c := range checkOrder {
		_, _ = h.Write([]byte(c.rule))
	}
	_, _ = h.Write([]byte(policyVersion))
	return "sha256:" + hex.EncodeToString(h.Sum(nil))[:16]
}

func checkCapability(r Request) result {
	if r.Pack.Empty() {
		return stop("the run has no capability pack")
	}
	if !r.Pack.Allows(r.Tool) {
		return stop(fmt.Sprintf("tool %q is outside the run's capability pack", r.Tool))
	}
	return pass()
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
		return stop("would exceed the run's " + dim + " ceiling")
	}
	return pass()
}

func checkIdempotency(r Request) result {
	if r.AlreadyExecuted {
		return stop("this exact call is already recorded; replaying it would duplicate the effect")
	}
	return pass()
}
