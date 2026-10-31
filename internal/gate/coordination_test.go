package gate

import (
	"testing"

	"github.com/fuseone/agents/internal/domain"
)

/*
The work-item claim is an internal coordination operation (#206).

It changes only the platform's record of who works on what. As an ordinary
write it would park for a person before reserving the card, and the race it
exists to close would reopen — so it passes without approval, tainted or not.
Everything else here is about keeping that exemption to exactly that call.
*/

func claimRequest(stage domain.Stage, labels ...string) Request {
	r := request()
	r.Tool, r.Effect = domain.ToolWorkItemClaim, domain.EffectWrite
	r.Args = []byte(`{"ref":"github:fuseone-io/fuseone-agents#123"}`)
	r.Pack = NewPack(domain.ToolWorkItemClaim, "crm.note")
	r.Stage = stage
	r.ArgLabels = domain.NewLabels(labels...)
	return r
}

func TestEvaluate_workItemClaimInATaintedRun_passesWithoutApproval(t *testing.T) {
	t.Parallel()

	for _, stage := range []domain.Stage{domain.StageCopilot, domain.StageAutonomous} {
		d := evaluate(t, New(), claimRequest(stage, domain.LabelUntrusted))
		if d.Verdict != domain.VerdictAllow || d.Rule != RulePassed {
			t.Errorf("%s: claim after an untrusted read = %s/%s (%s), want allow/passed",
				stage, d.Verdict, d.Rule, d.Reason)
		}
	}
}

// The control beside it: the same tainted copilot run still parks its
// ordinary writes. The exemption is the claim's, not the run's.
func TestEvaluate_ordinaryWriteInTheSameTaintedRun_stillNeedsApproval(t *testing.T) {
	t.Parallel()

	r := claimRequest(domain.StageCopilot, domain.LabelUntrusted)
	r.Tool = "crm.note"
	if d := evaluate(t, New(), r); d.Verdict != domain.VerdictRequireApproval {
		t.Fatalf("tainted ordinary write = %s/%s, want require_approval", d.Verdict, d.Rule)
	}
}

// Closed by name. Another platform-owned write, and a near miss of the id,
// keep the rules every write lives under.
func TestEvaluate_coordinationExemption_isClosedToEveryOtherTool(t *testing.T) {
	t.Parallel()

	for _, tool := range []domain.ToolID{
		domain.ToolMemorySuggest, domain.ToolTicketAnswer,
		"$fuseone.work_item.clam", "github.claim",
	} {
		r := claimRequest(domain.StageCopilot, domain.LabelUntrusted)
		r.Tool = tool
		r.Pack = NewPack(tool)
		if d := evaluate(t, New(), r); d.Verdict != domain.VerdictRequireApproval {
			t.Errorf("%s: tainted write = %s/%s, want require_approval", tool, d.Verdict, d.Rule)
		}
	}
}

// Never a read: a claim's answer depends on every other open run at this
// moment, so a classification that would let it be served from a cache is a
// wiring mistake the Gate refuses rather than trusts.
func TestEvaluate_workItemClaimClassifiedAsAnythingButAWrite_blocked(t *testing.T) {
	t.Parallel()

	for _, effect := range []domain.Effect{domain.EffectRead, domain.EffectDestructive} {
		r := claimRequest(domain.StageAutonomous)
		r.Effect = effect
		if d := evaluate(t, New(), r); d.Verdict != domain.VerdictBlock || d.Rule != RuleContract {
			t.Errorf("claim classified %s = %s/%s, want block/contract", effect, d.Verdict, d.Rule)
		}
	}
}

// Offered only to agents that declare it: the pack still decides.
func TestEvaluate_workItemClaimOutsideThePack_blocked(t *testing.T) {
	t.Parallel()

	r := claimRequest(domain.StageAutonomous)
	r.Pack = NewPack("crm.note")
	if d := evaluate(t, New(), r); d.Verdict != domain.VerdictBlock || d.Rule != RuleCapability {
		t.Fatalf("undeclared claim = %s/%s, want block/capability", d.Verdict, d.Rule)
	}
}

// A draft never acts unsupervised, coordination included.
func TestEvaluate_workItemClaimByADraft_stillNeedsApproval(t *testing.T) {
	t.Parallel()

	if d := evaluate(t, New(), claimRequest(domain.StageDraft)); d.Verdict != domain.VerdictRequireApproval {
		t.Fatalf("draft claim = %s/%s, want require_approval", d.Verdict, d.Rule)
	}
}

// The built-in ladder is lowered; an operator's own rule is not overridden.
func TestEvaluate_workItemClaimDeniedByAnAuthoredPolicy_blocked(t *testing.T) {
	t.Parallel()

	g := New().WithPolicies(Policies{Hash: "pol_test", Set: []domain.Policy{{
		Code: "POL-NO-CLAIM", Resource: string(domain.ToolWorkItemClaim),
		Reach: domain.ReachInstallation, Effect: domain.PolicyDeny,
		Mode: domain.PolicyEnforce, Enabled: true,
	}}})
	if d := evaluate(t, g, claimRequest(domain.StageAutonomous)); d.Verdict != domain.VerdictBlock {
		t.Fatalf("claim under an authored deny = %s/%s, want block", d.Verdict, d.Rule)
	}
}
