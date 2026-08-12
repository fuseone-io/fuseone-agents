package policy_test

import (
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/policy"
)

// A simulation is read by somebody deciding whether to turn a rule on. Every
// way it can mislead is a way somebody enforces something they did not mean to.

func decided(tool string, effect domain.Effect, was domain.Verdict) domain.RecordedDecision {
	return domain.RecordedDecision{
		RunID: domain.RunID("run-" + tool), Seq: 3,
		Scope: domain.Scope{Company: "acme", Area: "cx"}, AgentID: "triage",
		Tool: domain.ToolID(tool), Effect: effect, Verdict: was,
	}
}

func draftRule(over func(*domain.Policy)) domain.Policy {
	p := domain.Policy{
		Code: "POL-500", Name: "rascunho", Resource: "crm.*",
		Reach: domain.ReachInstallation, Effect: domain.PolicyDeny,
		Mode: domain.PolicyEnforce, Enabled: true,
	}
	if over != nil {
		over(&p)
	}
	return p
}

func TestSimulate_countsWhatTheRuleWouldHaveApplyTo(t *testing.T) {
	t.Parallel()

	got := policy.Simulate(draftRule(nil), []domain.RecordedDecision{
		decided("crm.reply", domain.EffectWrite, domain.VerdictAllow),
		decided("crm.lookup", domain.EffectRead, domain.VerdictAllow),
		decided("kb.search", domain.EffectRead, domain.VerdictAllow),
	})

	if got.Considered != 3 {
		t.Errorf("considered = %d, want 3", got.Considered)
	}
	if got.Matched != 2 {
		t.Errorf("matched = %d, want the two crm calls", got.Matched)
	}
	if got.ByVerdict[domain.VerdictBlock] != 2 {
		t.Errorf("blocks = %d, want 2", got.ByVerdict[domain.VerdictBlock])
	}
}

func TestSimulate_ruleReadingArguments_reportsUnknownRatherThanNoMatch(t *testing.T) {
	t.Parallel()

	// The failure that matters. A blocked call never stored arguments, so a
	// rule reading them cannot be replayed — and answering "no matches" would
	// show zero denials for exactly the rule somebody was nervous about.
	got := policy.Simulate(draftRule(func(p *domain.Policy) {
		p.Conditions = []domain.Condition{
			{Field: "args.rows", Op: domain.OpGreaterThan, Value: "100"},
		}
	}), []domain.RecordedDecision{
		decided("crm.reply", domain.EffectWrite, domain.VerdictAllow),
	})

	if got.Matched != 0 {
		t.Errorf("matched = %d, want none claimed", got.Matched)
	}
	if got.Unknown != 1 {
		t.Errorf("unknown = %d, want the decision reported as unanswerable", got.Unknown)
	}
}

func TestSimulate_decisionTheRuleDoesNotCover_isANoNotAnUnknown(t *testing.T) {
	t.Parallel()

	// A rule that does not reach this tool at all is answerable: the answer
	// is no. Counting it as unknown would make every simulation look uncertain.
	got := policy.Simulate(draftRule(func(p *domain.Policy) {
		p.Resource = "crm.*"
		p.Conditions = []domain.Condition{
			{Field: "args.rows", Op: domain.OpGreaterThan, Value: "100"},
		}
	}), []domain.RecordedDecision{
		decided("kb.search", domain.EffectRead, domain.VerdictAllow),
	})

	if got.Unknown != 0 {
		t.Errorf("unknown = %d, want none — the rule does not reach that tool", got.Unknown)
	}
	if got.Matched != 0 {
		t.Errorf("matched = %d, want none", got.Matched)
	}
}

func TestSimulate_carriesSampleRunsSoANumberHasSomethingBehindIt(t *testing.T) {
	t.Parallel()

	got := policy.Simulate(draftRule(nil), []domain.RecordedDecision{
		decided("crm.reply", domain.EffectWrite, domain.VerdictAllow),
	})

	if len(got.Samples) != 1 {
		t.Fatalf("samples = %+v, want one", got.Samples)
	}
	// What it was and what it would become, side by side: the change is the
	// fact, not the new verdict on its own.
	if got.Samples[0].Was != domain.VerdictAllow || got.Samples[0].WouldBe != domain.VerdictBlock {
		t.Errorf("sample = %+v, want allow becoming block", got.Samples[0])
	}
}

func TestSimulate_manyMatches_keepsTheAnswerReadable(t *testing.T) {
	t.Parallel()

	var decisions []domain.RecordedDecision
	for range 50 {
		decisions = append(decisions, decided("crm.reply", domain.EffectWrite, domain.VerdictAllow))
	}

	got := policy.Simulate(draftRule(nil), decisions)
	if got.Matched != 50 {
		t.Errorf("matched = %d, want all of them counted", got.Matched)
	}
	// A reader checks three. Fifty is a second problem rather than evidence.
	if len(got.Samples) > 3 {
		t.Errorf("samples = %d, want at most three", len(got.Samples))
	}
}

func TestSimulate_nothingRecorded_saysSoRatherThanLookingSafe(t *testing.T) {
	t.Parallel()

	got := policy.Simulate(draftRule(nil), nil)

	// Zero matches out of zero decisions is not evidence a rule is harmless,
	// and Considered is what lets the screen say which of the two it is.
	if got.Considered != 0 || got.Matched != 0 {
		t.Errorf("simulation = %+v, want an empty one", got)
	}
}
