package gate_test

import (
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/gate"
)

// An agent in Copilot acts only with a person's approval, whatever the policy
// would otherwise allow. It is the stage where an installation finds out
// whether it agrees with the agent, on real work, at no risk.

// allowing is a written exception for the tool, which is the one thing that
// lowers the built-in floor. Copilot has to survive it, or the stage would
// mean nothing on exactly the agents somebody wrote an exception for.
func allowing() *gate.Gate {
	return gate.New().WithPolicies(gate.Policies{Set: []domain.Policy{{
		Code: "POL-020", Enabled: true, Resource: "crm.reply",
		Effect: domain.PolicyAllow, Reach: domain.ReachInstallation,
	}}})
}

func TestEvaluate_copilot_escalatesEvenWhatAWrittenExceptionAllows(t *testing.T) {
	t.Parallel()

	got, err := allowing().Evaluate(t.Context(), gate.Request{
		Tool: "crm.reply", Effect: domain.EffectWrite, Stage: domain.StageCopilot,
		Pack: gate.NewPack("crm.reply"), Args: []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}

	if got.Verdict != domain.VerdictRequireApproval {
		t.Fatalf("verdict = %v, want a person asked", got.Verdict)
	}
	// Named, so an operator reading the trail knows the stage caused it
	// rather than hunting for a policy that does not exist.
	if got.Rule != gate.RuleAutonomy {
		t.Errorf("rule = %q, want the stage named", got.Rule)
	}
}

func TestEvaluate_copilot_letsAReadThrough(t *testing.T) {
	t.Parallel()

	// A copilot that asked permission to look something up would have a person
	// clicking through the whole run, and a person clicking through is a
	// person not reading.
	got := ruleOn(t, gate.Request{
		Tool: "crm.lookup", Effect: domain.EffectRead, Stage: domain.StageCopilot,
		Pack: gate.NewPack("crm.lookup"), Args: []byte(`{}`),
	})

	if got.Verdict != domain.VerdictAllow {
		t.Errorf("verdict = %v, want the read allowed", got.Verdict)
	}
}

func TestEvaluate_autonomous_isJudgedByTheWrittenExceptionAlone(t *testing.T) {
	t.Parallel()

	// The same call, the same exception, one stage further: this is the whole
	// difference promotion makes, and the only place it is visible.
	got, err := allowing().Evaluate(t.Context(), gate.Request{
		Tool: "crm.reply", Effect: domain.EffectWrite, Stage: domain.StageAutonomous,
		Pack: gate.NewPack("crm.reply"), Args: []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}

	if got.Verdict != domain.VerdictAllow {
		t.Errorf("verdict = %v, want the exception to hold", got.Verdict)
	}
}

func TestEvaluate_anUnsetStage_actsAsDraftAndAsksAnyway(t *testing.T) {
	t.Parallel()

	// A request with no stage is a wiring mistake, and the safe reading of a
	// wiring mistake is the least trusted one.
	got := ruleOn(t, gate.Request{
		Tool: "crm.reply", Effect: domain.EffectWrite,
		Pack: gate.NewPack("crm.reply"), Args: []byte(`{}`),
	})

	if got.Verdict == domain.VerdictAllow {
		t.Errorf("verdict = %v, want an unstaged agent not acting alone", got.Verdict)
	}
}

func ruleOn(t *testing.T, r gate.Request) domain.Decision {
	t.Helper()
	got, err := gate.New().Evaluate(t.Context(), r)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	return got
}

func TestEvaluate_aCompensation_undoesWhatWasAllowed(t *testing.T) {
	t.Parallel()

	// The undo is not in the pack, because the author never chose it: the
	// Curator decided what undoes what. The permission it borrows is exactly
	// the one that let the original call happen.
	got := ruleOn(t, gate.Request{
		Tool: "crm.charge.refund", Effect: domain.EffectFinancial,
		Compensating: "crm.charge", Stage: domain.StageAutonomous,
		Pack: gate.NewPack("crm.charge"), Args: []byte(`{}`),
	})

	if got.Rule == gate.RuleCapability {
		t.Fatalf("refused on capability: %+v", got)
	}
}

func TestEvaluate_aCompensationForSomethingOutsideThePack_isStillRefused(t *testing.T) {
	t.Parallel()

	// Otherwise "compensating" would be a word that opens the pack: anything
	// could be called by claiming to undo something the agent never had.
	got := ruleOn(t, gate.Request{
		Tool: "crm.anything", Effect: domain.EffectWrite,
		Compensating: "crm.never", Stage: domain.StageAutonomous,
		Pack: gate.NewPack("crm.charge"), Args: []byte(`{}`),
	})

	if got.Verdict != domain.VerdictBlock || got.Rule != gate.RuleCapability {
		t.Errorf("decision = %+v, want it refused on capability", got)
	}
}

func TestEvaluate_aFinancialCompensation_isNotHeldByTheBuiltInFloor(t *testing.T) {
	t.Parallel()

	// The floor denies financial effects so an agent cannot invent one nobody
	// authorised. A refund of a charge that already crossed the Gate is not
	// that: it is a person undoing an act the installation permitted. Holding
	// it here would mean the acts most worth undoing are the ones that never
	// can be.
	got := ruleOn(t, gate.Request{
		Tool: "crm.charge.refund", Effect: domain.EffectFinancial,
		Compensating: "crm.charge", Stage: domain.StageAutonomous,
		Pack: gate.NewPack("crm.charge"), Args: []byte(`{}`),
		ApprovalGranted: true,
	})

	if !got.Verdict.Executable() {
		t.Errorf("decision = %+v, want the refund allowed to run", got)
	}
}

func TestEvaluate_anAuthoredDenyStillStopsACompensation(t *testing.T) {
	t.Parallel()

	// Lowering the built-in floor is not the same as ignoring the rules an
	// installation wrote. If somebody denied this tool by name, it stays
	// denied — compensation included.
	g := gate.New().WithPolicies(gate.Policies{Set: []domain.Policy{{
		Code: "POL-900", Enabled: true, Resource: "crm.charge.refund",
		Reason: "no refunds by machine",
		Effect: domain.PolicyDeny, Reach: domain.ReachInstallation,
	}}})

	got, err := g.Evaluate(t.Context(), gate.Request{
		Scope: domain.Scope{Company: "acme", Area: "billing"},
		RunID: "run-1", AgentID: "billing", Seq: 1,
		Tool: "crm.charge.refund", Effect: domain.EffectFinancial,
		Compensating: "crm.charge", Stage: domain.StageAutonomous,
		Pack: gate.NewPack("crm.charge"), Args: []byte(`{}`),
		ApprovalGranted: true,
	})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if got.Verdict != domain.VerdictBlock {
		t.Errorf("decision = %+v, want it blocked by the authored rule", got)
	}
}
