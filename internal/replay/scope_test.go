package replay_test

import (
	"context"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/gate"
	"github.com/fuseone/agents/internal/replay"
)

/*
A replay asks the scope question the run asked, from where the tool was then
(NT-019). A step decided before the Gate judged scope is replayed by
TestRun_aTrailThatSaysWhatTheRulesWouldSay_isFaithful, which records none.
*/
func TestRun_aScopeDecision_isReplayedFromTheRecordedToolScope(t *testing.T) {
	t.Parallel()
	finance := domain.RecordScope(domain.Scope{Company: "acme", Area: "finance"})

	for _, one := range []struct {
		name     string
		verdict  domain.Verdict
		rule     string
		faithful bool
	}{
		{"blocked as it was", domain.VerdictBlock, gate.RuleScope, true},
		// The tamper the chain cannot see: a sibling area's tool recorded as
		// allowed. The rules would have stopped it.
		{"allowed against the rule", domain.VerdictAllow, gate.RulePassed, false},
	} {
		t.Run(one.name, func(t *testing.T) {
			t.Parallel()
			steps := trail(t, domain.GateDecidedPayload{
				Tool: "crm.lookup", Effect: domain.EffectRead,
				Verdict: one.verdict, Rule: one.rule,
				Stage: domain.StageAutonomous, ToolScope: finance,
			}, "pol_empty")

			report, err := replay.Run(context.Background(), steps,
				snapshots{"pol_empty": nil}, pack(gate.NewPack("crm.lookup")))
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			if report.Faithful() != one.faithful {
				t.Errorf("faithful = %v, want %v: %+v", report.Faithful(), one.faithful, report)
			}
		})
	}
}

/*
A decision made under other built-in checks is not compared (NT-019).

The policy hash finds the authored set, and says nothing about the checks
around it. A decision recorded under another built-in version cannot be asked
again: today's checks are not the ones that ruled. It is reported as such,
with the reason, and never counted as reproduced or as diverged.
*/
func TestRun_aDecisionUnderOtherBuiltinChecks_isReportedNotCompared(t *testing.T) {
	t.Parallel()
	steps := trail(t, domain.GateDecidedPayload{
		Tool: "crm.lookup", Effect: domain.EffectRead,
		Verdict: domain.VerdictAllow, Rule: gate.RulePassed,
		Stage: domain.StageAutonomous, Builtin: "sha256:0000000000000000",
	}, "pol_empty")

	report, err := replay.Run(context.Background(), steps,
		snapshots{"pol_empty": nil}, pack(gate.NewPack("crm.lookup")))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if report.Reproduced != 0 || len(report.Divergences) != 1 || report.Divergences[0].Why == "" {
		t.Fatalf("report = %+v, want one decision reported as not comparable", report)
	}
}

// Under the same built-in checks, the decision is compared as any other.
func TestRun_aDecisionUnderTodaysBuiltinChecks_isCompared(t *testing.T) {
	t.Parallel()
	steps := trail(t, domain.GateDecidedPayload{
		Tool: "crm.lookup", Effect: domain.EffectRead,
		Verdict: domain.VerdictAllow, Rule: gate.RulePassed,
		Stage: domain.StageAutonomous, Builtin: gate.BuiltinVersion(),
	}, "pol_empty")

	report, err := replay.Run(context.Background(), steps,
		snapshots{"pol_empty": nil}, pack(gate.NewPack("crm.lookup")))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !report.Faithful() {
		t.Errorf("report = %+v, want it reproduced", report)
	}
}
