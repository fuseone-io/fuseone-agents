package gate

import (
	"testing"

	"github.com/fuseone/agents/internal/domain"
)

/*
The work-item link is a coordination operation only when it is proven (#204,
slice 2b).

The engine verifies the proof against the run's own record before asking —
the run's create_pull_request returned, and its answer named this pull request
— and says so on the request. Proven, the link passes like the claim, tainted
or not. Unproven, it is an ordinary write: it parks for a person, and that
person's approval is what the ledger then records as its proof.
*/

func linkRequest(stage domain.Stage, proven bool, labels ...string) Request {
	r := request()
	r.Tool, r.Effect = domain.ToolWorkItemLink, domain.EffectWrite
	r.Args = []byte(`{"ref":"github:fuseone-io/fuseone-agents#240"}`)
	r.Pack = NewPack(domain.ToolWorkItemLink, domain.ToolWorkItemClaim, "crm.note")
	r.Stage = stage
	r.ArgLabels = domain.NewLabels(labels...)
	r.LinkProven = proven
	return r
}

func TestEvaluate_provenLinkAfterAnUntrustedRead_passesWithoutApproval(t *testing.T) {
	t.Parallel()

	for _, stage := range []domain.Stage{domain.StageCopilot, domain.StageAutonomous} {
		d := evaluate(t, New(), linkRequest(stage, true, domain.LabelUntrusted))
		if d.Verdict != domain.VerdictAllow || d.Rule != RulePassed {
			t.Errorf("%s: proven link after an untrusted read = %s/%s (%s), want allow/passed",
				stage, d.Verdict, d.Rule, d.Reason)
		}
	}
}

// The other half of the same rule: without the proof the link is a write like
// any other, in every stage and whether or not the run is tainted.
func TestEvaluate_unprovenLink_parksForApproval(t *testing.T) {
	t.Parallel()

	for _, stage := range []domain.Stage{domain.StageCopilot, domain.StageAutonomous} {
		for _, labels := range [][]string{{domain.LabelUntrusted}, nil} {
			d := evaluate(t, New(), linkRequest(stage, false, labels...))
			if d.Verdict != domain.VerdictRequireApproval {
				t.Errorf("%s %v: unproven link = %s/%s (%s), want require_approval",
					stage, labels, d.Verdict, d.Rule, d.Reason)
			}
		}
	}
}

// Approved by a person, the unproven link goes through: the decision is its
// proof.
func TestEvaluate_unprovenLinkApproved_allowedByApproval(t *testing.T) {
	t.Parallel()

	r := linkRequest(domain.StageCopilot, false, domain.LabelUntrusted)
	r.ApprovalGranted = true
	if d := evaluate(t, New(), r); d.Verdict != domain.VerdictAllow || d.Rule != RuleApproval {
		t.Fatalf("approved unproven link = %s/%s, want allow/approval", d.Verdict, d.Rule)
	}
}

// Proof is the link's alone. The claim needs none, and no other tool borrows
// the exemption by carrying the flag.
func TestEvaluate_linkProvenFlagOnAnotherTool_exemptsNothing(t *testing.T) {
	t.Parallel()

	for _, tool := range []domain.ToolID{
		domain.ToolMemorySuggest, domain.ToolTicketAnswer, "github.create_pull_request", "$fuseone.work_item.lnk",
	} {
		r := linkRequest(domain.StageCopilot, true, domain.LabelUntrusted)
		r.Tool, r.Pack = tool, NewPack(tool)
		if d := evaluate(t, New(), r); d.Verdict != domain.VerdictRequireApproval {
			t.Errorf("%s with LinkProven: tainted write = %s/%s, want require_approval", tool, d.Verdict, d.Rule)
		}
	}
	claim := claimRequest(domain.StageCopilot, domain.LabelUntrusted)
	if d := evaluate(t, New(), claim); d.Verdict != domain.VerdictAllow {
		t.Fatalf("the claim without LinkProven = %s/%s; the claim needs no proof", d.Verdict, d.Rule)
	}
}

func TestEvaluate_linkClassifiedAsAnythingButAWrite_blocked(t *testing.T) {
	t.Parallel()

	for _, proven := range []bool{true, false} {
		for _, effect := range []domain.Effect{domain.EffectRead, domain.EffectDestructive, domain.EffectFinancial} {
			r := linkRequest(domain.StageAutonomous, proven)
			r.Effect = effect
			if d := evaluate(t, New(), r); d.Verdict != domain.VerdictBlock || d.Rule != RuleContract {
				t.Errorf("link (proven %v) classified %s = %s/%s, want block/contract", proven, effect, d.Verdict, d.Rule)
			}
		}
	}
}

// A link the engine already knows cannot be made — no card held, a reference
// outside the card's repository — is refused, never parked: a person could
// only approve something the ledger would refuse. It wins over a proof.
func TestEvaluate_linkThatCannotBeMade_blockedNotParked(t *testing.T) {
	t.Parallel()

	for _, proven := range []bool{true, false} {
		r := linkRequest(domain.StageCopilot, proven, domain.LabelUntrusted)
		r.LinkRefusal = "the run holds no card"
		d := evaluate(t, New(), r)
		if d.Verdict != domain.VerdictBlock || d.Rule != RuleContract || d.Reason != "the run holds no card" {
			t.Errorf("unmakeable link (proven %v) = %s/%s (%s), want block/contract with the reason",
				proven, d.Verdict, d.Rule, d.Reason)
		}
	}
	// The refusal is the link's: on any other tool it changes nothing.
	r := linkRequest(domain.StageAutonomous, false)
	r.Tool, r.Pack, r.LinkRefusal = "crm.note", NewPack("crm.note"), "the run holds no card"
	if d := evaluate(t, New(), r); d.Verdict == domain.VerdictBlock {
		t.Fatalf("LinkRefusal on another tool blocked it: %s/%s", d.Verdict, d.Rule)
	}
}

func TestEvaluate_provenLinkOutsideThePack_blocked(t *testing.T) {
	t.Parallel()

	r := linkRequest(domain.StageAutonomous, true)
	r.Pack = NewPack(domain.ToolWorkItemClaim)
	if d := evaluate(t, New(), r); d.Verdict != domain.VerdictBlock || d.Rule != RuleCapability {
		t.Fatalf("undeclared link = %s/%s, want block/capability", d.Verdict, d.Rule)
	}
}

func TestEvaluate_provenLinkByADraft_stillNeedsApproval(t *testing.T) {
	t.Parallel()

	if d := evaluate(t, New(), linkRequest(domain.StageDraft, true)); d.Verdict != domain.VerdictRequireApproval {
		t.Fatalf("draft link = %s/%s, want require_approval", d.Verdict, d.Rule)
	}
}

// The meaning of the built-in checks changed, so the version that names them
// did: v6 for links, v7 for the scope check (NT-019). A decision taken under
// one must not read as one taken under the other.
func TestPolicyVersion_isV7(t *testing.T) {
	t.Parallel()

	if policyVersion != "builtin/v7" {
		t.Fatalf("policyVersion = %q, want builtin/v7", policyVersion)
	}
}
