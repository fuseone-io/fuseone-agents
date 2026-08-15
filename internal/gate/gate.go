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
	// RuleAutonomy is an agent not yet trusted to act alone. Named in the
	// trail so an operator reading an escalation knows the stage caused it
	// rather than hunting for a policy that does not exist.
	RuleAutonomy = "autonomy"
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
	// Last on purpose. It escalates everything an untrusted agent would do,
	// so running it early would report "the agent is in Copilot" for a call
	// that a policy or a taint rule was already stopping for a specific
	// reason — and the specific reason is the one somebody can act on. When
	// nothing else objected, this is the explanation.
	{RuleAutonomy, checkAutonomy},
}

// Gate evaluates requests against the seven checks and the authored policies.
//
// Approval is not among them. It is what a check can require, and a human
// grant is released after all seven have run — the only order in which a
// grant cannot let through what another check blocked.
type Gate struct {
	policyHash string
	policies   []domain.Policy
}

func New() *Gate {
	return &Gate{policyHash: builtinPolicyHash()}
}

// WithPolicies gives the Gate the set in force and the hash that names it.
//
// A snapshot, never a database: the Gate is on the path of every effect and
// must not depend on a query succeeding to decide whether something is
// allowed. Whoever holds it refreshes it; a stale set decides consistently
// and the hash on the step says which set that was.
func (g *Gate) WithPolicies(p Policies) *Gate {
	return &Gate{policyHash: p.Hash, policies: p.Set}
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

	// Run once, before the loop, because two checks need the answer: the
	// policy check itself, and the ladder it may excuse.
	authored := evaluatePolicies(g.policies, inputFrom(r))
	worst.Monitored = monitoredFrom(authored)

	for _, c := range checkOrder {
		got := c.eval(r)
		if c.rule == RulePolicy {
			got = mergePolicy(got, authored)
		}
		if got.verdict == domain.VerdictAllow {
			continue
		}
		d := domain.Decision{
			Verdict:    got.verdict,
			Rule:       c.rule,
			Reason:     got.reason,
			PolicyHash: g.policyHash,
			Monitored:  worst.Monitored,
		}
		if c.rule == RulePolicy {
			// Which policy, not just "policy". Without it the trail cannot
			// tell two rules apart and nobody can count what either did.
			d.PolicyCode = authored.code
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
			Monitored:  worst.Monitored,
		}, nil
	}
	return worst, nil
}

// mergePolicy decides what the policy check says once the authored set has
// been read.
//
// The ladder is the floor. An authored policy that matched and said allow is
// the one thing that lowers it — that is what an exception is, and somebody's
// name is on it. Everything else can only tighten.
func mergePolicy(ladder result, authored policyResult) result {
	if authored.verdict > domain.VerdictAllow {
		return result{verdict: authored.verdict, reason: authored.reason}
	}
	if authored.allowed {
		return pass()
	}
	return ladder
}

// monitoredFrom carries the watching policies onto the decision.
func monitoredFrom(authored policyResult) []domain.MonitoredPolicy {
	if len(authored.monitors) == 0 {
		return nil
	}
	out := make([]domain.MonitoredPolicy, 0, len(authored.monitors))
	for _, m := range authored.monitors {
		out = append(out, domain.MonitoredPolicy{
			Code: m.Code, Verdict: m.Verdict, Reason: m.Reason,
		})
	}
	return out
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
