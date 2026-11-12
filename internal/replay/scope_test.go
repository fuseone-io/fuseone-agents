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
