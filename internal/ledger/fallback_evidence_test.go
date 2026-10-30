package ledger_test

import (
	"context"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
)

/*
A run that switched to its fallback is evidence about a pair the version was
not promoted on (#27). The aggregates promotion reads leave it out, in both
stores: a fake that counted it would certify a Trust Center production does
not have.

Each case seeds one run on the declared pair and one that fell back, with the
same evidence in both, and asserts only the declared run is counted — so the
broken version, which counts both, cannot pass.
*/
func TestFallbackRunsContract(t *testing.T) {
	base := time.Date(2026, 10, 30, 9, 0, 0, 0, time.UTC)
	window := domain.RunFilter{AgentID: "triage", VersionID: "v3", Since: base.Add(-time.Hour)}

	seed := func(t *testing.T, s Store, id domain.RunID, fellBack bool) {
		t.Helper()
		mustAppend(t, s, startedAt(id, base))
		if fellBack {
			switched := step(id, domain.StepModelFallback)
			switched.At = base.Add(time.Second)
			switched.Payload = mustJSON(t, domain.ModelFallbackPayload{
				From:    domain.ModelPair{Provider: "anthropic", Model: "claude-opus-5"},
				To:      domain.ModelPair{Provider: "openai", Model: "gpt-test"},
				Failure: domain.FailureSummary{Code: "model_provider_overloaded", Retryable: true},
			})
			mustAppend(t, s, switched)
		}
		blocked := step(id, domain.StepGateDecided)
		blocked.At = base.Add(2 * time.Second)
		blocked.Payload = mustJSON(t, domain.GateDecidedPayload{
			Tool: "crm.reply", Verdict: domain.VerdictBlock, Rule: "policy",
		})
		mustAppend(t, s, blocked)
		decided := step(id, domain.StepApprovalDecided)
		decided.At = base.Add(3 * time.Second)
		decided.Payload = mustJSON(t, domain.ApprovalDecidedPayload{Approved: true, By: "ana"})
		mustAppend(t, s, decided)
	}
	both := func(t *testing.T, s Store) {
		t.Helper()
		seed(t, s, "run-declared", false)
		seed(t, s, "run-fallback", true)
	}

	run(t, "agreement leaves out runs that fell back", func(t *testing.T, s Store) {
		both(t, s)
		got, err := s.Agreement(context.Background(), base.Add(-time.Hour))
		if err != nil {
			t.Fatalf("Agreement: %v", err)
		}
		if len(got) != 1 || got[0].Approved != 1 {
			t.Errorf("agreement = %+v, want the declared run's approval alone", got)
		}
	})

	run(t, "version agreement leaves out runs that fell back", func(t *testing.T, s Store) {
		both(t, s)
		got, err := s.VersionAgreement(context.Background(), window)
		if err != nil {
			t.Fatalf("VersionAgreement: %v", err)
		}
		if len(got) != 1 || got[0].Approved != 1 {
			t.Errorf("agreement = %+v, want the declared run's approval alone", got)
		}
	})

	run(t, "version gate blocks leave out runs that fell back", func(t *testing.T, s Store) {
		both(t, s)
		got, err := s.VersionGateBlocks(context.Background(), window)
		if err != nil {
			t.Fatalf("VersionGateBlocks: %v", err)
		}
		if len(got) != 1 || got[0].Blocks != 1 || got[0].Runs != 1 {
			t.Errorf("blocks = %+v, want the declared run's block alone", got)
		}
	})

	run(t, "activity read as evidence leaves out runs that fell back", func(t *testing.T, s Store) {
		both(t, s)
		evidence := window
		evidence.DeclaredPairOnly = true
		got, err := s.AgentActivity(context.Background(), evidence)
		if err != nil {
			t.Fatalf("AgentActivity: %v", err)
		}
		if len(got) != 1 || got[0].Runs != 1 {
			t.Errorf("activity = %+v, want the declared run alone", got)
		}
	})

	run(t, "activity read as a listing still counts every run", func(t *testing.T, s Store) {
		// The agent list reports what the agent did and what it cost. A run
		// that fell back still ran and still spent.
		both(t, s)
		got, err := s.AgentActivity(context.Background(), window)
		if err != nil {
			t.Fatalf("AgentActivity: %v", err)
		}
		if len(got) != 1 || got[0].Runs != 2 {
			t.Errorf("activity = %+v, want both runs", got)
		}
	})
}
