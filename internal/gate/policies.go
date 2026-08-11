package gate

import (
	"github.com/fuseone/agents/internal/domain"
)

// Authored policies, and how they meet the built-in ladder.
//
// The ladder is the floor: an installation that has authored nothing still
// refuses destructive and financial effects and still asks a human before a
// write. Losing that by adding a policy engine would make the feature a
// downgrade for every installation that has not used it yet.
//
// An authored policy that *matches* and says allow is the one thing that
// lowers the floor, because that is what an exception is: somebody wrote it,
// their name is on it, and its conditions had to hold. Everything else can
// only tighten.

// Policies is the set in force, declared here by the consumer. The Gate never
// reads a database; it is given a snapshot and the hash that names it.
type Policies struct {
	Hash string
	Set  []domain.Policy
}

// Monitored is a policy that matched while watching rather than enforcing.
//
// Recorded so an operator can read what a rule would have done before turning
// it on, and so the trail can say that a decision was observed and not obeyed
// — otherwise the screen shows a rule denying things and a run that carried
// on, and somebody spends an afternoon on it.
type Monitored struct {
	Code    string
	Verdict domain.Verdict
	Reason  string
}

// policyResult is what the authored set said about one call.
type policyResult struct {
	// verdict is the most restrictive among enforcing policies that matched.
	verdict domain.Verdict
	code    string
	reason  string
	// allowed is true when an enforcing policy matched and said allow. It is
	// separate from verdict because "nothing objected" and "somebody wrote an
	// exception" are different facts, and only the second lowers the floor.
	allowed  bool
	monitors []Monitored
}

// evaluatePolicies runs the authored set over one call.
func evaluatePolicies(set []domain.Policy, in domain.PolicyInput) policyResult {
	out := policyResult{verdict: domain.VerdictAllow}

	for _, p := range set {
		if !p.Matches(in) {
			continue
		}
		verdict := p.Effect.Verdict()

		if p.Mode == domain.PolicyMonitor {
			// Evaluated, recorded, obeyed by nothing.
			out.monitors = append(out.monitors, Monitored{
				Code: p.Code, Verdict: verdict, Reason: p.Reason,
			})
			continue
		}

		if p.Effect == domain.PolicyAllow {
			out.allowed = true
			if out.code == "" {
				out.code, out.reason = p.Code, p.Reason
			}
			continue
		}
		if verdict > out.verdict {
			out.verdict, out.code, out.reason = verdict, p.Code, p.Reason
		}
	}
	return out
}

// inputFrom projects a request into what a policy may read.
//
// A projection rather than the request itself: a policy must not be able to
// read the idempotency key or the budget internals, and a type that offered
// them would eventually have a rule written against one.
func inputFrom(r Request) domain.PolicyInput {
	return domain.PolicyInput{
		Tool:   r.Tool,
		Effect: r.Effect,
		Agent:  r.AgentID,
		Scope:  r.Scope,
		Labels: r.ArgLabels,
		Args:   r.Args,
	}
}
