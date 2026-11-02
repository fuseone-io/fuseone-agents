package ledger_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/fuseone/agents/internal/domain"
)

/*
A work_item_linked step, against both stores (#204, slice 2a).

A link is sealed only when its payload is one every reader can read, and a run
with a link in it goes through every reader that walks steps by kind exactly
as it did before. The writer's fence — the held claim, the proof, one root per
alias — is in workitem_alias_test.go (2b).
*/

const linkAlias = domain.WorkItemRef("github:fuseone-io/fuseone-agents#240")

func linkPayload(proofSeq int64) domain.WorkItemLinkedPayload {
	return domain.WorkItemLinkedPayload{
		Alias: linkAlias, Root: card,
		Proof: domain.WorkItemLinkProof{Kind: domain.StepToolReturned, Seq: proofSeq},
	}
}

func linkStepOf(t *testing.T, runID domain.RunID, agent domain.AgentID, payload any) domain.Step {
	t.Helper()
	linked := step(runID, domain.StepWorkItemLinked)
	linked.AgentID = agent
	linked.Payload = mustJSON(t, payload)
	return linked
}

// claimedAndReturned opens run-a, claims the card and records a returned call
// for the link to name as its proof. It returns that step's seq.
func claimedAndReturned(t *testing.T, s Store) int64 {
	t.Helper()
	openRunOf(t, s, "run-a", "developer")
	if err := claim(t, s, "run-a", "developer", card); err != nil {
		t.Fatalf("claim: %v", err)
	}
	called := withPayload(t, step("run-a", domain.StepToolCalled),
		domain.ToolCalledPayload{Tool: "github.create_pull_request", Effect: domain.EffectWrite})
	called.AgentID = "developer"
	mustAppend(t, s, called)
	returned := withPayload(t, step("run-a", domain.StepToolReturned), domain.ToolReturnedPayload{
		Tool: "github.create_pull_request", ResultRef: "content:pr", PullRequest: linkAlias,
	})
	returned.AgentID = "developer"
	sealed, err := s.Append(context.Background(), returned)
	if err != nil {
		t.Fatalf("return the call: %v", err)
	}
	return sealed.Seq
}

func TestWorkItemLink_validPayload_sealed(t *testing.T) {
	run(t, "a valid link is sealed", func(t *testing.T, s Store) {
		proof := claimedAndReturned(t, s)

		sealed, err := s.Append(context.Background(), linkStepOf(t, "run-a", "developer", linkPayload(proof)))
		if err != nil {
			t.Fatalf("Append(valid link) = %v; this release must read what the next one writes", err)
		}
		if sealed.Kind != domain.StepWorkItemLinked || sealed.Seq != proof+1 {
			t.Fatalf("sealed = %s at %d, want the link at %d", sealed.Kind, sealed.Seq, proof+1)
		}
		if err := s.Verify(context.Background(), "run-a"); err != nil {
			t.Fatalf("Verify after a link: %v", err)
		}
	})
}

func TestWorkItemLink_invalidPayload_refusedAndNotAStep(t *testing.T) {
	cases := map[string]func(proof int64) any{
		"missing alias": func(p int64) any { l := linkPayload(p); l.Alias = ""; return l },
		"missing root":  func(p int64) any { l := linkPayload(p); l.Root = ""; return l },
		"alias is root": func(p int64) any { l := linkPayload(p); l.Alias = card; return l },
		"another repo": func(p int64) any {
			l := linkPayload(p)
			l.Alias = "github:fuseone-io/fuseone-site#240"
			return l
		},
		"missing proof seq": func(p int64) any { l := linkPayload(p); l.Proof.Seq = 0; return l },
		// The proof is an earlier step of the run, never the link itself or
		// one not yet written.
		"proof is the link itself": func(p int64) any { return linkPayload(p + 1) },
		"proof not yet written":    func(p int64) any { return linkPayload(p + 7) },
		"not a payload at all":     func(int64) any { return []string{"github:fuseone-io/fuseone-agents#240"} },
		"no payload":               func(int64) any { return nil },
	}
	for name, payload := range cases {
		run(t, "a link with "+name+" is refused", func(t *testing.T, s Store) {
			proof := claimedAndReturned(t, s)
			before := headSeq(t, s, "run-a")

			_, err := s.Append(context.Background(), linkStepOf(t, "run-a", "developer", payload(proof)))
			if !errors.Is(err, domain.ErrWorkItemLinkInvalid) {
				t.Fatalf("Append(link with %s) = %v, want ErrWorkItemLinkInvalid", name, err)
			}
			if after := headSeq(t, s, "run-a"); after != before {
				t.Fatalf("a refused link moved the head from %d to %d", before, after)
			}
		})
	}
}

// Every reader that walks a run by kind meets the link and carries the run as
// before: its phase, its place on the card's trail, its claim, its opening
// and closing steps, its count in the stats.
func TestWorkItemLink_aRunWithALink_readsAsItDidWithoutOne(t *testing.T) {
	run(t, "readers tolerate a link", func(t *testing.T, s Store) {
		ctx := context.Background()
		proof := claimedAndReturned(t, s)
		mustAppend(t, s, linkStepOf(t, "run-a", "developer", linkPayload(proof)))

		runs, err := s.ListRuns(ctx, domain.RunFilter{}, "", 10)
		if err != nil || len(runs) != 1 || runs[0].Phase != "running" {
			t.Fatalf("ListRuns = %+v, %v; want run-a still running", runs, err)
		}
		finished := step("run-a", domain.StepRunFinished)
		finished.AgentID = "developer"
		mustAppend(t, s, finished)

		trail := trailOf(t, s, domain.RunFilter{}, nil, domain.WorkItemPage{})
		if !slices.Equal(trailIDs(trail.Runs), []domain.RunID{"run-a"}) || trail.Runs[0].Phase != "finished" {
			t.Fatalf("trail of the root = %+v, want run-a, finished, as its claim put it there", trail.Runs)
		}

		ends, err := s.StartsAndFinishes(ctx, asked(t, s, "run-a"))
		if err != nil {
			t.Fatalf("StartsAndFinishes: %v", err)
		}
		if got := ends["run-a"]; len(got) != 2 || got[0].Kind != domain.StepRunStarted || got[1].Kind != domain.StepRunFinished {
			t.Fatalf("StartsAndFinishes = %+v, want the opening and closing steps alone", got)
		}

		stats, err := s.Stats(ctx, domain.RunFilter{})
		if err != nil || stats.Total != 1 || stats.ByPhase["finished"] != 1 {
			t.Fatalf("Stats = %+v, %v; want one finished run", stats, err)
		}
	})
}

// A link does not take a card's place: the run still holds its claim, and
// another open run of the agent is still refused the card.
func TestWorkItemLink_doesNotMoveTheClaim(t *testing.T) {
	run(t, "the claim stands after a link", func(t *testing.T, s Store) {
		proof := claimedAndReturned(t, s)
		mustAppend(t, s, linkStepOf(t, "run-a", "developer", linkPayload(proof)))

		if err := claim(t, s, "run-a", "developer", card); err != nil {
			t.Fatalf("retry of the run's own claim after a link = %v, want success", err)
		}
		openRunOf(t, s, "run-b", "developer")
		if err := claim(t, s, "run-b", "developer", card); !errors.Is(err, domain.ErrWorkItemHeld) {
			t.Fatalf("second run's claim of the root = %v, want ErrWorkItemHeld", err)
		}
	})
}

// A simulation's run reads a link the way a real one does.
func TestWorkItemLink_inASimulatedRun_sealed(t *testing.T) {
	run(t, "a simulated run's link", func(t *testing.T, s Store) {
		openSimulatedRun(t, s, "run-sim", "developer")
		if err := claim(t, s, "run-sim", "developer", card); err != nil {
			t.Fatalf("simulated claim: %v", err)
		}
		proof := returnPullRequest(t, s, "run-sim", "developer", linkAlias)
		if _, err := s.Append(context.Background(), linkStepOf(t, "run-sim", "developer", linkPayload(proof))); err != nil {
			t.Fatalf("Append(simulated link) = %v", err)
		}
	})
}
