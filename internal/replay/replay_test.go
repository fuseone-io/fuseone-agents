package replay_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/replay"
)

// Counterfactual replay answers "what would this policy have changed?". The
// only way it can be wrong in a way that matters is by answering "nothing"
// about a decision it could not actually re-evaluate.

func decided(seq int64, tool string, verdict domain.Verdict, labels ...string) domain.Step {
	payload, _ := json.Marshal(domain.GateDecidedPayload{
		Tool: domain.ToolID(tool), Effect: domain.EffectWrite,
		Verdict: verdict, Rule: "passed", Labels: domain.NewLabels(labels...),
	})
	return domain.Step{
		RunID: "run-1", Seq: seq, Kind: domain.StepGateDecided,
		Scope: domain.Scope{Company: "acme", Area: "cx"}, AgentID: "suporte",
		At: time.Now(), Payload: payload,
	}
}

func denying(field, value string) domain.Policy {
	return domain.Policy{
		Code: "POL-1", Enabled: true, Effect: domain.PolicyDeny,
		Conditions: []domain.Condition{{Field: field, Op: "eq", Value: value}},
	}
}

func TestAgainst_reportsADecisionTheNewRuleWouldChange(t *testing.T) {
	t.Parallel()

	got := replay.Against(
		[]domain.Step{decided(3, "crm.refund", domain.VerdictAllow)},
		[]domain.Policy{denying("tool.id", "crm.refund")},
	)

	if len(got.Changed) != 1 {
		t.Fatalf("changed = %+v, want the refund decision", got.Changed)
	}
	if got.Changed[0].Was != domain.VerdictAllow || got.Changed[0].Now != domain.VerdictBlock {
		t.Errorf("difference = %+v", got.Changed[0])
	}
}

func TestAgainst_leavesADecisionTheRuleDoesNotTouch(t *testing.T) {
	t.Parallel()

	got := replay.Against(
		[]domain.Step{decided(3, "crm.lookup", domain.VerdictAllow)},
		[]domain.Policy{denying("tool.id", "crm.refund")},
	)

	if len(got.Changed) != 0 {
		t.Errorf("changed = %+v, want none", got.Changed)
	}
	if got.Evaluated != 1 {
		t.Errorf("evaluated = %d", got.Evaluated)
	}
}

func TestAgainst_aRuleReadingArgumentContent_isReportedUnevaluable(t *testing.T) {
	t.Parallel()

	// The arguments are not in the ledger — deliberately, because they carry
	// whatever the case carried. So this decision cannot be re-evaluated, and
	// the one thing the report must not do is call it unchanged: somebody is
	// about to publish a policy on the strength of this answer.
	got := replay.Against(
		[]domain.Step{decided(3, "crm.refund", domain.VerdictAllow)},
		[]domain.Policy{denying("args.rows", "500")},
	)

	if len(got.Changed) != 0 {
		t.Errorf("changed = %+v, want nothing claimed", got.Changed)
	}
	if len(got.Unevaluable) != 1 {
		t.Fatalf("unevaluable = %+v, want the decision reported as unanswerable", got.Unevaluable)
	}
	if got.Evaluated != 0 {
		t.Errorf("evaluated = %d, want it not counted as evaluated", got.Evaluated)
	}
}

func TestAgainst_anArgumentRuleThatCannotApply_doesNotBlockTheAnswer(t *testing.T) {
	t.Parallel()

	// The rule reads arguments but is scoped to another tool, so it could
	// never have applied to this decision. Refusing to answer here would make
	// the report useless the moment an installation has one such rule.
	rule := denying("args.rows", "500")
	rule.Resource = "crm.refund"

	got := replay.Against(
		[]domain.Step{decided(3, "crm.lookup", domain.VerdictAllow)},
		[]domain.Policy{rule},
	)

	if len(got.Unevaluable) != 0 {
		t.Errorf("unevaluable = %+v, want none", got.Unevaluable)
	}
	if got.Evaluated != 1 {
		t.Errorf("evaluated = %d", got.Evaluated)
	}
}

func TestAgainst_aTaintRule_isEvaluableBecauseLabelsAreRecorded(t *testing.T) {
	t.Parallel()

	got := replay.Against(
		[]domain.Step{decided(3, "crm.reply", domain.VerdictAllow, domain.LabelUntrusted)},
		[]domain.Policy{denying("data.taint", domain.LabelUntrusted)},
	)

	if len(got.Changed) != 1 {
		t.Fatalf("changed = %+v, want the tainted decision", got.Changed)
	}
}

func TestAgainst_ignoresEverythingThatIsNotADecision(t *testing.T) {
	t.Parallel()

	steps := []domain.Step{
		{RunID: "run-1", Seq: 1, Kind: domain.StepRunStarted},
		decided(2, "crm.lookup", domain.VerdictAllow),
		{RunID: "run-1", Seq: 3, Kind: domain.StepToolCalled},
	}
	got := replay.Against(steps, []domain.Policy{denying("tool.id", "crm.lookup")})

	if got.Evaluated != 1 || len(got.Changed) != 1 {
		t.Errorf("report = %+v", got)
	}
}
