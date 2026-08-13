package replay_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/gate"
	"github.com/fuseone/agents/internal/replay"
)

/*
The hash chain proves the steps were not edited. It does not prove they were
ever the correct output of the rules the platform says were in force — a chain
of well-sealed lies verifies perfectly. These are about the other half.
*/

type snapshots map[string][]domain.Policy

func (s snapshots) Snapshot(_ context.Context, hash string) ([]domain.Policy, error) {
	set, ok := s[hash]
	if !ok {
		return nil, errNoSnapshot
	}
	return set, nil
}

var errNoSnapshot = &notKept{}

type notKept struct{}

func (*notKept) Error() string { return "no snapshot" }

type pack gate.Pack

func (p pack) Pack(context.Context, domain.AgentID, domain.VersionID) (gate.Pack, error) {
	return gate.Pack(p), nil
}

// trail seals a run that read something and then decided about a write.
func trail(t *testing.T, decision domain.GateDecidedPayload, hash string) []domain.Step {
	t.Helper()

	at := time.Date(2026, 8, 13, 9, 0, 0, 0, time.UTC)
	specs := []domain.Step{
		{
			Kind: domain.StepRunStarted, Payload: mustJSON(t, domain.RunStartedPayload{Trigger: "cron"}),
		},
		{
			Kind: domain.StepGateDecided, PolicyHash: hash, Payload: mustJSON(t, decision),
		},
	}

	var out []domain.Step
	var prev *domain.Step
	for i, spec := range specs {
		spec.RunID = "run-1"
		spec.Scope = domain.Scope{Company: "acme", Area: "cx"}
		spec.AgentID, spec.VersionID = "suporte", "v1"
		spec.At = at.Add(time.Duration(i) * time.Second)
		sealed, err := domain.NewStep(prev, spec)
		if err != nil {
			t.Fatalf("NewStep: %v", err)
		}
		out = append(out, sealed)
		prev = &out[len(out)-1]
	}
	return out
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

func TestRun_aTrailThatSaysWhatTheRulesWouldSay_isFaithful(t *testing.T) {
	t.Parallel()

	// A read inside the pack, allowed. Replayed under the same empty policy
	// set, the Gate has to reach the same answer for the same reason.
	steps := trail(t, domain.GateDecidedPayload{
		Tool: "crm.lookup", Effect: domain.EffectRead,
		Verdict: domain.VerdictAllow, Rule: gate.RulePassed,
		Stage: domain.StageAutonomous,
	}, "pol_empty")

	report, err := replay.Run(context.Background(), steps,
		snapshots{"pol_empty": nil}, pack(gate.NewPack("crm.lookup")))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if !report.Faithful() {
		t.Errorf("report = %+v, want every decision reproduced", report)
	}
}

func TestRun_aVerdictNothingWouldProduce_isReported(t *testing.T) {
	t.Parallel()

	// The tamper the chain cannot see: a write recorded as allowed, sealed
	// correctly, under a policy set that would never have allowed it.
	steps := trail(t, domain.GateDecidedPayload{
		Tool: "crm.reply", Effect: domain.EffectWrite,
		Verdict: domain.VerdictAllow, Rule: gate.RulePassed,
		Stage: domain.StageAutonomous,
	}, "pol_empty")

	report, err := replay.Run(context.Background(), steps,
		snapshots{"pol_empty": nil}, pack(gate.NewPack("crm.reply")))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if report.Faithful() {
		t.Fatal("a write allowed against the built-in floor replayed as faithful")
	}
	if len(report.Divergences) != 1 || report.Divergences[0].Was != domain.VerdictAllow {
		t.Errorf("divergences = %+v, want the allow reported", report.Divergences)
	}
}

func TestRun_aPolicySetNobodyKept_isReportedRatherThanGuessed(t *testing.T) {
	t.Parallel()

	// Not a divergence and not a match. Calling it either would be a lie, and
	// the one that matters is calling it a match.
	steps := trail(t, domain.GateDecidedPayload{
		Tool: "crm.lookup", Effect: domain.EffectRead,
		Verdict: domain.VerdictAllow, Rule: gate.RulePassed,
		Stage: domain.StageAutonomous,
	}, "pol_gone")

	report, err := replay.Run(context.Background(), steps,
		snapshots{}, pack(gate.NewPack("crm.lookup")))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if report.Reproduced != 0 || len(report.Divergences) != 1 {
		t.Fatalf("report = %+v, want the missing snapshot reported", report)
	}
	if report.Divergences[0].Why == "" {
		t.Error("the report does not say why the decision could not be re-derived")
	}
}

func TestRun_aDecisionFromBeforeTheStageWasRecorded_saysSoRatherThanDiverging(t *testing.T) {
	t.Parallel()

	// A trail written by an older version of this platform. Replaying it under
	// an unset stage refuses everything, and reporting that as a divergence
	// would be blaming the record for a gap in what was recorded.
	steps := trail(t, domain.GateDecidedPayload{
		Tool: "crm.lookup", Effect: domain.EffectRead,
		Verdict: domain.VerdictAllow, Rule: gate.RulePassed,
	}, "pol_empty")

	report, err := replay.Run(context.Background(), steps,
		snapshots{"pol_empty": nil}, pack(gate.NewPack("crm.lookup")))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(report.Divergences) != 1 || report.Divergences[0].Why == "" {
		t.Errorf("divergences = %+v, want it reported as not re-derivable", report.Divergences)
	}
	if report.Divergences[0].Now != domain.VerdictUnknown {
		t.Error("the report invented an answer for a decision it could not replay")
	}
}
