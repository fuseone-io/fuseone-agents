package e2e_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/gate"
	"github.com/fuseone/agents/internal/replay"
)

/*
Faithful replay of a run this platform actually ran (PRD AU-07).

The unit tests build their own steps. This one takes the decisions a real Gate
made during a real run and asks the Gate to make them again — which is where a
mismatch between what the runner records and what the replay feeds back would
show up, and only there.
*/

type noPolicies struct{}

func (noPolicies) Snapshot(context.Context, string) ([]domain.Policy, error) {
	return nil, nil
}

type packOf gate.Pack

func (p packOf) Pack(context.Context, domain.AgentID, domain.VersionID) (gate.Pack, error) {
	return gate.Pack(p), nil
}

func TestReplay_ofARunThisPlatformRan_reproducesEveryDecision(t *testing.T) {
	eachLedger(t, "a real run's decisions come out the same way", func(t *testing.T, store Store) {
		p := newPlatform(t, store, agentFull, readThenAnswer)
		if err := p.catalog.Classify(domain.ToolClassification{
			Tool: "crm.lookup", Effect: domain.EffectRead,
		}); err != nil {
			t.Fatalf("classify: %v", err)
		}
		p.open(t, "run-replay-1")
		p.settle(t, "run-replay-1")

		steps := p.steps(t, "run-replay-1")
		report, err := replay.Run(t.Context(), steps, noPolicies{},
			packOf(gate.NewPack("crm.lookup", "crm.note")))
		if err != nil {
			t.Fatalf("Run: %v", err)
		}

		if report.Decisions == 0 {
			t.Fatal("the run recorded no gate decisions; the replay proves nothing")
		}
		if !report.Faithful() {
			t.Errorf("report = %+v, want every decision reproduced", report)
		}
	})
}

func TestReplay_aVerdictSomebodyChanged_isCaughtThoughTheChainVerifies(t *testing.T) {
	eachLedger(t, "an edited verdict survives verification and fails the replay", func(t *testing.T, store Store) {
		p := newPlatform(t, store, agentFull, writeThenStall)
		if err := p.catalog.Classify(domain.ToolClassification{
			Tool: "crm.note", Effect: domain.EffectWrite,
		}); err != nil {
			t.Fatalf("classify: %v", err)
		}
		p.open(t, "run-replay-2")
		p.settle(t, "run-replay-2")

		// The tamper the hash chain cannot see. Not an edit to a stored step —
		// the chain would catch that — but the record as it would look if the
		// platform had written the wrong answer in the first place: a write
		// escalated by the built-in floor, recorded as allowed.
		steps := p.steps(t, "run-replay-2")
		forged := false
		for i, step := range steps {
			if step.Kind != domain.StepGateDecided {
				continue
			}
			var decided domain.GateDecidedPayload
			if err := json.Unmarshal(step.Payload, &decided); err != nil {
				t.Fatalf("decode: %v", err)
			}
			decided.Verdict, decided.Rule = domain.VerdictAllow, gate.RulePassed
			steps[i].Payload = mustPayload(t, decided)
			forged = true
			break
		}
		if !forged {
			t.Fatal("the run recorded no decision to forge")
		}

		report, err := replay.Run(t.Context(), steps, noPolicies{},
			packOf(gate.NewPack("crm.note")))
		if err != nil {
			t.Fatalf("Run: %v", err)
		}

		if report.Faithful() {
			t.Fatal("a verdict the rules would never give replayed as faithful")
		}
		if len(report.Divergences) == 0 || report.Divergences[0].Was != domain.VerdictAllow {
			t.Errorf("divergences = %+v, want the forged allow named", report.Divergences)
		}
	})
}

func mustPayload(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}
