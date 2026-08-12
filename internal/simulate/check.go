package simulate

import "github.com/fuseone/agents/internal/domain"

/*
Check reports which expectations a case did not meet.

It returns the unmet ones rather than a message about each, because the
expectation already says what it wanted: "never calls crm.refund" unmet needs
no sentence explaining it, and a sentence would need translating.

Read against the fold of the run rather than against anything kept beside it,
so a regression case is checked against exactly what the trail says happened —
the same rows a person would read if they went and looked.
*/
func Check(c Case, want []domain.Expectation) []domain.Expectation {
	var unmet []domain.Expectation
	for _, e := range want {
		if !met(c, e) {
			unmet = append(unmet, e)
		}
	}
	return unmet
}

func met(c Case, e domain.Expectation) bool {
	// A case that never opened a run meets nothing. Counting it as passing
	// would make a green battery mean "nothing was checked" on the day that
	// matters most.
	if c.Error != "" && c.RunID == "" {
		return false
	}

	switch e.Kind {
	case domain.ExpectSettles:
		return string(c.Settled) == e.Value

	case domain.ExpectCalls:
		return c.any(e, func(a Act) bool {
			return string(a.Tool) == e.Value && a.Reached
		})

	case domain.ExpectNeverCalls:
		return !c.any(e, func(a Act) bool {
			return string(a.Tool) == e.Value && a.Reached
		})

	case domain.ExpectAsks:
		// Proposed and escalated, never reached: what the author asked for is
		// that a person decide, and the person deciding is the point.
		return c.any(e, func(a Act) bool {
			if e.Value != "" && string(a.Tool) != e.Value {
				return false
			}
			return a.Verdict == domain.VerdictRequireApproval
		})

	default:
		// An expectation nothing understands is not quietly satisfied. A
		// version of the platform older than the correction must fail the
		// battery rather than pass it.
		return false
	}
}

// any reports whether an act matches, within the step the expectation is
// anchored to. An expectation with no step is about the run as a whole, which
// is the only sensible anchor for an agent that declares no steps.
func (c Case) any(e domain.Expectation, match func(Act) bool) bool {
	for _, act := range c.Acted {
		if e.Step != "" && act.Step != e.Step {
			continue
		}
		if match(act) {
			return true
		}
	}
	return false
}
