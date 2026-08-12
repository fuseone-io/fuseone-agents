package simulate_test

import (
	"encoding/json"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/simulate"
)

type entry struct {
	kind    domain.StepKind
	payload any
	cost    domain.Cost
}

func run(t *testing.T, entries ...entry) []domain.Step {
	t.Helper()

	out := make([]domain.Step, 0, len(entries))
	for i, e := range entries {
		step := domain.Step{
			RunID: "run_sim_1", Seq: int64(i + 1), Kind: e.kind, Cost: e.cost,
		}
		if e.payload != nil {
			raw, err := json.Marshal(e.payload)
			if err != nil {
				t.Fatalf("payload %d: %v", i+1, err)
			}
			step.Payload = raw
		}
		out = append(out, step)
	}
	return out
}

func started() entry {
	return entry{kind: domain.StepRunStarted, payload: domain.RunStartedPayload{
		Trigger: "simulation", Simulated: true,
	}}
}

func planned(node string) entry {
	return entry{kind: domain.StepPlanned, payload: domain.PlannedPayload{Node: node}}
}

func decided(tool string, verdict domain.Verdict, rule string) entry {
	return entry{kind: domain.StepGateDecided, payload: domain.GateDecidedPayload{
		Tool: domain.ToolID(tool), Effect: domain.EffectWrite, Verdict: verdict, Rule: rule,
	}}
}

func TestFold_aPolicyBlock_namesTheRuleThatDecidedIt(t *testing.T) {
	t.Parallel()

	// Every authored rule decides under the rule name "policy", so without the
	// code the report reads "blocked by policy" for all of them — the one
	// sentence the trail is not allowed to produce (AU-10).
	got, err := simulate.Fold(run(t,
		started(),
		planned("Consultar"),
		entry{kind: domain.StepGateDecided, payload: domain.GateDecidedPayload{
			Tool: "crm.lookup", Effect: domain.EffectRead, Verdict: domain.VerdictBlock,
			Rule: "policy", PolicyCode: "sem-dados-de-cliente",
		}},
		entry{kind: domain.StepParked, payload: domain.ParkedPayload{Reason: "no_progress"}},
	))
	if err != nil {
		t.Fatalf("fold: %v", err)
	}
	if got.Acted[0].Policy != "sem-dados-de-cliente" {
		t.Errorf("policy = %q, want the authored rule named", got.Acted[0].Policy)
	}
}

func called(tool string) entry {
	return entry{kind: domain.StepToolCalled, payload: domain.ToolCalledPayload{
		Tool: domain.ToolID(tool), Effect: domain.EffectWrite,
	}}
}

func TestFold_aBlockedProposalIsReportedWithTheRuleThatBlockedIt(t *testing.T) {
	t.Parallel()

	// The row that matters most is the one with no tool call behind it. A
	// report anchored on what executed would drop it entirely, and the whole
	// point of simulating is to see where the policy would have stopped.
	got, err := simulate.Fold(run(t,
		started(),
		planned("Responder"),
		decided("crm.refund", domain.VerdictBlock, "effect.destructive"),
		entry{kind: domain.StepParked, payload: domain.ParkedPayload{Reason: "blocked"}},
	))
	if err != nil {
		t.Fatalf("fold: %v", err)
	}

	if len(got.Acted) != 1 {
		t.Fatalf("acts = %d, want the blocked proposal", len(got.Acted))
	}
	act := got.Acted[0]
	if act.Tool != "crm.refund" || act.Verdict != domain.VerdictBlock {
		t.Errorf("act = %+v", act)
	}
	if act.Rule != "effect.destructive" {
		// "Blocked by policy" tells an author nothing about what to change.
		t.Errorf("rule = %q, want the rule that decided", act.Rule)
	}
	if act.Reached {
		t.Error("a blocked proposal is reported as having reached the tool layer")
	}
}

func TestFold_anActSaysWhetherItReachedTheToolLayer(t *testing.T) {
	t.Parallel()

	got, err := simulate.Fold(run(t,
		started(),
		planned("Consultar"),
		decided("crm.lookup", domain.VerdictAllow, ""),
		called("crm.lookup"),
		entry{kind: domain.StepToolReturned, payload: domain.ToolReturnedPayload{Tool: "crm.lookup"}},
		entry{kind: domain.StepRunFinished, payload: domain.RunFinishedPayload{Outcome: "respondido"}},
	))
	if err != nil {
		t.Fatalf("fold: %v", err)
	}

	if len(got.Acted) != 1 || !got.Acted[0].Reached {
		t.Fatalf("acts = %+v, want one that reached", got.Acted)
	}
	if got.Settled != simulate.SettledFinished || got.Outcome != "respondido" {
		t.Errorf("settled %q outcome %q", got.Settled, got.Outcome)
	}
}

func TestFold_theSameToolTwice_isTwoActs(t *testing.T) {
	t.Parallel()

	// Keyed by name, the second decision would overwrite the first and a run
	// that called the same tool twice would report calling it once.
	got, err := simulate.Fold(run(t,
		started(),
		planned("Consultar"),
		decided("crm.lookup", domain.VerdictAllow, ""),
		called("crm.lookup"),
		entry{kind: domain.StepToolReturned, payload: domain.ToolReturnedPayload{Tool: "crm.lookup"}},
		planned("Consultar"),
		decided("crm.lookup", domain.VerdictBlock, "policy.rate"),
		entry{kind: domain.StepParked, payload: domain.ParkedPayload{Reason: "blocked"}},
	))
	if err != nil {
		t.Fatalf("fold: %v", err)
	}

	if len(got.Acted) != 2 {
		t.Fatalf("acts = %d, want two", len(got.Acted))
	}
	if !got.Acted[0].Reached || got.Acted[1].Reached {
		t.Errorf("reached = %v then %v", got.Acted[0].Reached, got.Acted[1].Reached)
	}
}

func TestFold_anActCarriesTheStepItWasProposedIn(t *testing.T) {
	t.Parallel()

	// A correction anchors to a step (FU-13). A report that says only "it was
	// blocked" leaves the author hunting for where.
	got, err := simulate.Fold(run(t,
		started(),
		planned("Consultar"),
		decided("crm.lookup", domain.VerdictAllow, ""),
		called("crm.lookup"),
		entry{kind: domain.StepToolReturned, payload: domain.ToolReturnedPayload{Tool: "crm.lookup"}},
		planned("Responder"),
		decided("email.send", domain.VerdictRequireApproval, "effect.write"),
		entry{kind: domain.StepApprovalRequested, payload: domain.ApprovalRequestedPayload{
			Tool: "email.send", Rule: "effect.write",
		}},
	))
	if err != nil {
		t.Fatalf("fold: %v", err)
	}

	if got.Acted[0].Step != "Consultar" || got.Acted[1].Step != "Responder" {
		t.Errorf("steps = %q then %q", got.Acted[0].Step, got.Acted[1].Step)
	}
	// Waiting for a person is where the case ends, and the report says so.
	if got.Settled != simulate.SettledWaiting {
		t.Errorf("settled = %q, want a case waiting on somebody", got.Settled)
	}
}

func TestFold_costIsTheSumOfEveryStep(t *testing.T) {
	t.Parallel()

	got, err := simulate.Fold(run(t,
		started(),
		entry{kind: domain.StepPlanned, payload: domain.PlannedPayload{Node: "Consultar"},
			cost: domain.Cost{InputTokens: 900, OutputTokens: 30, Micros: 4100}},
		decided("crm.lookup", domain.VerdictAllow, ""),
		called("crm.lookup"),
		entry{kind: domain.StepToolReturned, payload: domain.ToolReturnedPayload{Tool: "crm.lookup"},
			cost: domain.Cost{Micros: 250}},
		entry{kind: domain.StepRunFinished, payload: domain.RunFinishedPayload{Outcome: "ok"}},
	))
	if err != nil {
		t.Fatalf("fold: %v", err)
	}

	// The split survives: an author asking why a case is expensive is asking
	// about tokens, not only about money (FO-08).
	want := domain.Cost{InputTokens: 900, OutputTokens: 30, Micros: 4350}
	if got.Cost != want {
		t.Errorf("cost = %+v, want %+v", got.Cost, want)
	}
}

func TestFold_aRunStillMidTurnHasNotSettled(t *testing.T) {
	t.Parallel()

	// Drive gives up on a planner that will not settle, and the case still
	// belongs in the report: dropping it would make the coverage a lie.
	got, err := simulate.Fold(run(t, started(), planned("Consultar")))
	if err != nil {
		t.Fatalf("fold: %v", err)
	}
	if got.Settled != simulate.SettledUnsettled {
		t.Errorf("settled = %q", got.Settled)
	}
}
