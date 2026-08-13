package gate

import (
	"context"
	"testing"

	"github.com/fuseone/agents/internal/domain"
)

func request() Request {
	return Request{
		Scope:    domain.Scope{Company: "acme", Area: "cx"},
		RunID:    "run-1",
		AgentID:  "triage",
		Seq:      7,
		Tool:     "crm.lookup",
		Effect:   domain.EffectRead,
		Args:     []byte(`{"id":"42"}`),
		Pack:     NewPack("crm.lookup", "crm.note", "crm.refund"),
		Budget:   domain.Budget{Micros: 500_000, ToolCalls: 40},
		Estimate: domain.Consumption{Micros: 20_000, ToolCalls: 1},
		IdemKey:  "run-1:7:crm.lookup:abcd",
		// Trusted to act alone: these are tests about the checks, and an
		// unstaged agent escalates everything before they get a chance.
		Stage: domain.StageAutonomous,
	}
}

func evaluate(t *testing.T, g *Gate, r Request) domain.Decision {
	t.Helper()
	d, err := g.Evaluate(context.Background(), r)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	return d
}

func TestEvaluate_readToolInsidePack_allowed(t *testing.T) {
	t.Parallel()

	d := evaluate(t, New(), request())

	if d.Verdict != domain.VerdictAllow {
		t.Errorf("Verdict = %v (%s), want allow", d.Verdict, d.Rule)
	}
}

func TestEvaluate_allowed_doesNotNameACheckAsIfItObjected(t *testing.T) {
	t.Parallel()

	d := evaluate(t, New(), request())

	// The rule is rendered in the audit trail next to the verdict. Reporting
	// "allow / capability" reads as though the capability check had something
	// to say, and the reader is left deciphering a contradiction.
	if d.Rule != RulePassed {
		t.Errorf("Rule = %q, want %q for an unobjected action", d.Rule, RulePassed)
	}
}

func TestEvaluate_toolOutsidePack_blocked(t *testing.T) {
	t.Parallel()

	r := request()
	r.Tool = "payments.transfer"

	d := evaluate(t, New(), r)

	// The capability set is frozen when the run starts and may only shrink.
	// A tool absent from it is not merely denied — it is unreachable.
	if d.Verdict != domain.VerdictBlock {
		t.Errorf("Verdict = %v, want block", d.Verdict)
	}
	if d.Rule != RuleCapability {
		t.Errorf("Rule = %q, want %q", d.Rule, RuleCapability)
	}
}

func TestEvaluate_unclassifiedTool_blocked(t *testing.T) {
	t.Parallel()

	r := request()
	r.Effect = domain.EffectUnknown

	d := evaluate(t, New(), r)

	// An unclassified tool never executes: the Curator has not said what it
	// does to the world (PRD DE-12, DE-13).
	if d.Verdict != domain.VerdictBlock {
		t.Errorf("Verdict = %v, want block for an unclassified tool", d.Verdict)
	}
}

func TestEvaluate_writeEffect_requiresApproval(t *testing.T) {
	t.Parallel()

	r := request()
	r.Tool, r.Effect = "crm.note", domain.EffectWrite

	d := evaluate(t, New(), r)

	if d.Verdict != domain.VerdictRequireApproval {
		t.Errorf("Verdict = %v, want require_approval", d.Verdict)
	}
}

func TestEvaluate_destructiveEffect_blockedByDefault(t *testing.T) {
	t.Parallel()

	r := request()
	r.Tool, r.Effect = "crm.refund", domain.EffectFinancial

	d := evaluate(t, New(), r)

	if d.Verdict != domain.VerdictBlock {
		t.Errorf("Verdict = %v, want block", d.Verdict)
	}
}

func TestEvaluate_reversibleWriteWithUntrustedArguments_goesToAHuman(t *testing.T) {
	t.Parallel()

	r := request()
	r.Tool, r.Effect = "crm.note", domain.EffectWrite
	r.ArgLabels = domain.NewLabels(domain.LabelUntrusted)

	d := evaluate(t, New(), r)

	// Nearly every useful run reads untrusted input and then writes: a support
	// agent reads the ticket, then leaves a note. Blocking that outright would
	// forbid the primary use case while claiming to secure it, so a tainted
	// reversible write escalates to a human instead (PRD SE-06).
	if d.Verdict != domain.VerdictRequireApproval {
		t.Errorf("Verdict = %v, want require_approval", d.Verdict)
	}
	if d.Rule != RuleTaint {
		t.Errorf("Rule = %q, want %q", d.Rule, RuleTaint)
	}
}

func TestEvaluate_irreversibleActionWithUntrustedArguments_blocked(t *testing.T) {
	t.Parallel()

	r := request()
	r.Tool, r.Effect = "crm.refund", domain.EffectFinancial
	r.ArgLabels = domain.NewLabels(domain.LabelUntrusted)

	d := evaluate(t, New(), r)

	// No human approval releases this: an action that cannot be undone must
	// never derive from data an attacker may have authored.
	if d.Verdict != domain.VerdictBlock {
		t.Errorf("Verdict = %v, want block", d.Verdict)
	}
	if d.Rule != RuleTaint {
		t.Errorf("Rule = %q, want %q — taint outranks the effect ladder here", d.Rule, RuleTaint)
	}
}

func TestEvaluate_readWithUntrustedArguments_stillAllowed(t *testing.T) {
	t.Parallel()

	r := request()
	r.ArgLabels = domain.NewLabels(domain.LabelUntrusted)

	d := evaluate(t, New(), r)

	// Reads are how the agent investigates; tainting them into paralysis makes
	// the platform useless. Only effects on the world are gated by taint.
	if d.Verdict != domain.VerdictAllow {
		t.Errorf("Verdict = %v, want allow", d.Verdict)
	}
}

func TestEvaluate_estimateExceedsRemainingBudget_blocked(t *testing.T) {
	t.Parallel()

	r := request()
	r.Committed = domain.Consumption{Micros: 490_000}
	r.Estimate = domain.Consumption{Micros: 20_000}

	d := evaluate(t, New(), r)

	// The check is against committed plus the estimate, before spending, not
	// against a total accumulated afterwards (PRD FO-01).
	if d.Verdict != domain.VerdictBlock {
		t.Errorf("Verdict = %v, want block", d.Verdict)
	}
	if d.Rule != RuleBudget {
		t.Errorf("Rule = %q, want %q", d.Rule, RuleBudget)
	}
}

func TestEvaluate_alreadyExecutedIdempotencyKey_blockedAsDuplicate(t *testing.T) {
	t.Parallel()

	r := request()
	r.AlreadyExecuted = true

	d := evaluate(t, New(), r)

	if d.Verdict != domain.VerdictBlock {
		t.Errorf("Verdict = %v, want block", d.Verdict)
	}
	if d.Rule != RuleIdempotency {
		t.Errorf("Rule = %q, want %q", d.Rule, RuleIdempotency)
	}
}

// Ordering is normative: cheap absolute checks run before expensive
// contextual ones, and the first block wins. A request that violates both
// capability and budget must be reported as a capability failure, because
// that is the one the operator has to fix.
func TestEvaluate_multipleViolations_reportsTheEarliestCheck(t *testing.T) {
	t.Parallel()

	r := request()
	r.Tool = "payments.transfer"
	r.Committed = domain.Consumption{Micros: 499_999}
	r.AlreadyExecuted = true

	d := evaluate(t, New(), r)

	if d.Rule != RuleCapability {
		t.Errorf("Rule = %q, want %q — check order is part of the contract", d.Rule, RuleCapability)
	}
}

func TestEvaluate_everyDecision_carriesPolicyHash(t *testing.T) {
	t.Parallel()

	g := New()
	for _, tc := range []struct {
		name string
		mut  func(*Request)
	}{
		{"allow", func(*Request) {}},
		{"block", func(r *Request) { r.Tool = "payments.transfer" }},
		{"approval", func(r *Request) { r.Tool, r.Effect = "crm.note", domain.EffectWrite }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := request()
			tc.mut(&r)

			d := evaluate(t, g, r)

			// Without the policy hash a replay can reproduce the outcome but
			// never re-evaluate it against a newer policy (PRD AU-08).
			if d.PolicyHash == "" {
				t.Error("PolicyHash is empty; counterfactual replay is impossible")
			}
			if d.Rule == "" {
				t.Error("Rule is empty; the trail would read 'denied by policy'")
			}
		})
	}
}

func TestPack_frozenSet_onlyEverShrinks(t *testing.T) {
	t.Parallel()

	p := NewPack("a", "b", "c")
	narrowed := p.Narrow(NewPack("b", "c", "d"))

	if narrowed.Allows("d") {
		t.Error("Narrow widened the capability set")
	}
	if !narrowed.Allows("b") || !narrowed.Allows("c") {
		t.Error("Narrow dropped a capability present in both sets")
	}
}
