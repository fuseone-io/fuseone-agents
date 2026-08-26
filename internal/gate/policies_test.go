package gate_test

import (
	"context"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/gate"
)

// The built-in ladder is the floor. Adding a policy engine must not make an
// installation that has authored nothing more permissive than it was.

func request(over func(*gate.Request)) gate.Request {
	r := gate.Request{
		Scope:   domain.Scope{Company: "acme", Area: "cx"},
		RunID:   "run-1",
		AgentID: "triage",
		Seq:     2,
		Tool:    "crm.reply",
		Effect:  domain.EffectWrite,
		Pack:    gate.NewPack("crm.reply", "crm.lookup", "crm.refund"),
		Budget:  domain.Budget{Micros: 1_000_000, ToolCalls: 20, Steps: 50},
		// Trusted to act alone, so these read as tests about the policy set
		// rather than about the agent's stage. A request with no stage
		// escalates every effect, which is the safe default and a different
		// subject.
		Stage: domain.StageAutonomous,
	}
	if over != nil {
		over(&r)
	}
	return r
}

func withPolicies(policies ...domain.Policy) *gate.Gate {
	return gate.New().WithPolicies(gate.Policies{Hash: "pol_test", Set: policies})
}

func authored(code string, over func(*domain.Policy)) domain.Policy {
	p := domain.Policy{
		Code: code, Name: code, Owner: "Governança", Reason: "porque sim",
		Resource: "*", Reach: domain.ReachInstallation,
		Effect: domain.PolicyDeny, Mode: domain.PolicyEnforce, Enabled: true,
	}
	if over != nil {
		over(&p)
	}
	return p
}

func decide(t *testing.T, g *gate.Gate, r gate.Request) domain.Decision {
	t.Helper()
	got, err := g.Evaluate(context.Background(), r)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	return got
}

func TestEvaluate_noAuthoredPolicies_keepsTheBuiltInLadder(t *testing.T) {
	t.Parallel()

	// The regression that would matter most: an installation that has authored
	// nothing must still ask a human before a write.
	got := decide(t, withPolicies(), request(nil))

	if got.Verdict != domain.VerdictRequireApproval {
		t.Errorf("verdict = %v, want the ladder's escalation for a write", got.Verdict)
	}
}

func TestEvaluate_authoredDeny_tightensTheLadder(t *testing.T) {
	t.Parallel()

	g := withPolicies(authored("POL-114", func(p *domain.Policy) { p.Resource = "crm.*" }))
	got := decide(t, g, request(nil))

	if got.Verdict != domain.VerdictBlock {
		t.Errorf("verdict = %v, want block", got.Verdict)
	}
	// Which policy, not just "policy". Without it nobody can count what a
	// rule did or tell two of them apart.
	if got.PolicyCode != "POL-114" {
		t.Errorf("policyCode = %q, want the rule that fired", got.PolicyCode)
	}
	if got.Reason != "porque sim" {
		t.Errorf("reason = %q, want the sentence the author wrote", got.Reason)
	}
}

func TestEvaluate_authoredAllow_isTheOneThingThatLowersTheFloor(t *testing.T) {
	t.Parallel()

	// That is what an exception is: somebody wrote it, their name is on it,
	// and its conditions had to hold.
	g := withPolicies(authored("POL-020", func(p *domain.Policy) {
		p.Resource = "crm.reply"
		p.Effect = domain.PolicyAllow
	}))

	got := decide(t, g, request(nil))
	if got.Verdict != domain.VerdictAllow {
		t.Errorf("verdict = %v, want the written exception to hold", got.Verdict)
	}
}

func TestEvaluate_allowWhoseConditionsDoNotHold_leavesTheFloorInPlace(t *testing.T) {
	t.Parallel()

	// An exception that did not apply is not an exception. The ladder is back.
	g := withPolicies(authored("POL-020", func(p *domain.Policy) {
		p.Resource = "crm.reply"
		p.Effect = domain.PolicyAllow
		p.Conditions = []domain.Condition{
			{Field: "args.reviewed", Op: domain.OpEquals, Value: "true"},
		}
	}))

	got := decide(t, g, request(nil))
	if got.Verdict != domain.VerdictRequireApproval {
		t.Errorf("verdict = %v, want the ladder back", got.Verdict)
	}
}

func TestEvaluate_pendingReviewWrite_stillObeysAuthoredDeny(t *testing.T) {
	t.Parallel()

	g := withPolicies(authored("POL-MEM", func(p *domain.Policy) {
		p.Resource = "$fuseone.memory.suggest"
	}))
	got := decide(t, g, request(func(r *gate.Request) {
		r.Tool = domain.ToolMemorySuggest
		r.Effect = domain.EffectWrite
		r.Pack = gate.NewPack(domain.ToolMemorySuggest)
		r.PendingReview = true
		r.ArgLabels = domain.NewLabels(domain.LabelUntrusted)
	}))

	if got.Verdict != domain.VerdictBlock || got.Rule != gate.RulePolicy {
		t.Fatalf("decision = %s/%s, want the authored deny to still win", got.Verdict, got.Rule)
	}
	if got.PolicyCode != "POL-MEM" {
		t.Fatalf("policyCode = %q, want POL-MEM", got.PolicyCode)
	}
}

func TestEvaluate_allowNeverBeatsADenyFromAnotherCheck(t *testing.T) {
	t.Parallel()

	// A policy governs policy. It does not excuse a tool that is not in the
	// pack — that is the capability check, and nothing loosens it.
	g := withPolicies(authored("POL-020", func(p *domain.Policy) {
		p.Effect = domain.PolicyAllow
	}))

	got := decide(t, g, request(func(r *gate.Request) { r.Tool = "erp.transfer" }))
	if got.Verdict != domain.VerdictBlock || got.Rule != "capability" {
		t.Errorf("decision = %v/%s, want the capability block", got.Verdict, got.Rule)
	}
}

func TestEvaluate_monitoringPolicy_isRecordedAndObeyedByNothing(t *testing.T) {
	t.Parallel()

	// The whole point of monitor mode: read what a rule would have done
	// before it does it.
	g := withPolicies(authored("POL-114", func(p *domain.Policy) {
		p.Resource = "crm.*"
		p.Mode = domain.PolicyMonitor
	}))

	got := decide(t, g, request(nil))

	if got.Verdict == domain.VerdictBlock {
		t.Error("a monitoring policy blocked the call")
	}
	if len(got.Monitored) != 1 || got.Monitored[0].Code != "POL-114" {
		t.Fatalf("monitored = %+v, want the watching rule recorded", got.Monitored)
	}
	// Recorded with what it would have done, or the trail cannot show it.
	if got.Monitored[0].Verdict != domain.VerdictBlock {
		t.Errorf("monitored verdict = %v, want the block it would have raised",
			got.Monitored[0].Verdict)
	}
}

func TestEvaluate_monitoringPolicy_survivesOntoABlockFromElsewhere(t *testing.T) {
	t.Parallel()

	// A block short-circuits the loop. The watching rules still have to reach
	// the trail, or turning a policy on becomes guesswork on exactly the runs
	// that were already going wrong.
	g := withPolicies(authored("POL-900", func(p *domain.Policy) {
		p.Mode = domain.PolicyMonitor
	}))

	got := decide(t, g, request(func(r *gate.Request) { r.Tool = "erp.transfer" }))

	if got.Verdict != domain.VerdictBlock {
		t.Fatalf("verdict = %v, want the capability block", got.Verdict)
	}
	if len(got.Monitored) != 1 {
		t.Errorf("monitored = %+v, want it recorded despite the block", got.Monitored)
	}
}

func TestEvaluate_disabledPolicy_saysNothingAtAll(t *testing.T) {
	t.Parallel()

	// Different from monitoring: a disabled rule produces no observation to
	// record, so the trail must not suggest it looked at anything.
	g := withPolicies(authored("POL-114", func(p *domain.Policy) {
		p.Resource = "crm.*"
		p.Enabled = false
	}))

	got := decide(t, g, request(nil))
	if got.Verdict == domain.VerdictBlock {
		t.Error("a disabled policy blocked the call")
	}
	if len(got.Monitored) != 0 {
		t.Errorf("monitored = %+v, want nothing from a disabled rule", got.Monitored)
	}
}

func TestEvaluate_theHashNamesTheSetThatDecided(t *testing.T) {
	t.Parallel()

	got := decide(t, withPolicies(authored("POL-114", nil)), request(nil))

	// The seal has to name the authored set, not the built-in default, or the
	// decision cannot be reconstructed under the rules that produced it.
	if got.PolicyHash != "pol_test" {
		t.Errorf("policyHash = %q, want the snapshot's", got.PolicyHash)
	}
}
