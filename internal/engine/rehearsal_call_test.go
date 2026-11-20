package engine

import (
	"context"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/gate"
)

/*
A rehearsal's new call waits for its author (NT-018 phase 4, review of #429). Release 4a is
the rollback floor for the one that opens rehearsals, so it must park a rehearsal's calls
itself: an ordinary tool, the engine's own work-item claim and link, and a call a person (or
the rehearsal's author) approved. None of them reaches any tool, the coordination ledger or
the semantic dedupe store.
*/

func rehearsalHarness(t *testing.T, proposals ...Proposal) (*harness, Start) {
	t.Helper()
	h := newHarness(t, proposals...)
	start := h.start(t, generousBudget())
	start.Rehearsal = true
	start.Pack = gate.NewPack("crm.lookup", "crm.note", "crm.refund", domain.ToolWorkItemClaim, domain.ToolWorkItemLink)
	start.Stage = domain.StageCopilot
	openRunWithInput(t, h, start, "rehearsal", []byte("a case the author typed"), domain.ScopeLabels(start.Scope))
	return h, start
}

func headOf(t *testing.T, h *harness, start Start) domain.Step {
	t.Helper()
	head, err := h.ledger.Head(context.Background(), start.RunID)
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	return head
}

func TestAdvance_aRehearsalsNewCall_waitsForTheAuthorAndReachesNothing(t *testing.T) {
	t.Parallel()
	for name, p := range map[string]Proposal{
		"a tool":              {Tool: "crm.lookup", Args: []byte(`{"id":"c-1"}`)},
		"the work-item claim": claimProposal(claimedCard),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h, start := rehearsalHarness(t, p)
			if _, err := h.runner.Advance(context.Background(), start); err != nil {
				t.Fatalf("Advance: %v", err)
			}
			if head := headOf(t, h, start); head.Kind != domain.StepWorldAwaited || len(h.tools.invocations) != 0 {
				t.Errorf("head %s, %d calls; want world_awaited and nothing called", head.Kind, len(h.tools.invocations))
			}
			if claims := h.stepsOf(t, start.RunID, domain.StepWorkItemClaimed); len(claims) != 0 {
				t.Errorf("a rehearsal claimed a real card: %+v", claims)
			}
		})
	}
}

// A rehearsal never holds a card, since its claim waits for the author instead of reaching
// the ledger, so its link is refused by the contract before any call is made.
func TestAdvance_aRehearsalsLink_isRefusedForTheCardItNeverHeld(t *testing.T) {
	t.Parallel()
	h, start := rehearsalHarness(t, linkProposal("github:fuseone-io/fuseone-agents#240"))
	if _, err := h.runner.Advance(context.Background(), start); err != nil {
		t.Fatalf("Advance: %v", err)
	}
	if links := h.stepsOf(t, start.RunID, domain.StepWorkItemLinked); len(links) != 0 || len(h.tools.invocations) != 0 {
		t.Errorf("links %d, calls %d; want neither", len(links), len(h.tools.invocations))
	}
	if called := h.stepsOf(t, start.RunID, domain.StepToolCalled); len(called) != 0 {
		t.Errorf("the link reached a call: %+v", called)
	}
}

// The semantic dedupe store is real state shared with production runs: a rehearsal reads
// none of it and writes none of it.
func TestAdvance_aRehearsalsCall_touchesNoSemanticDedupe(t *testing.T) {
	t.Parallel()
	h, start := rehearsalHarness(t, Proposal{Tool: "crm.lookup", Args: []byte(`{"id":"c-1"}`)})
	store := &fakeDedupeStore{}
	enableDedupe(h, store)
	if _, err := h.runner.Advance(context.Background(), start); err != nil {
		t.Fatalf("Advance: %v", err)
	}
	if len(store.lookups)+len(store.reserves)+len(store.confirms)+len(store.releases) != 0 {
		t.Errorf("dedupe: %d lookups, %d reserves, %d confirms, %d releases; want none",
			len(store.lookups), len(store.reserves), len(store.confirms), len(store.releases))
	}
	if head := headOf(t, h, start); head.Kind != domain.StepWorldAwaited {
		t.Errorf("head %s, want world_awaited", head.Kind)
	}
}

// An approved call is still a call: in a rehearsal it waits for the author's answer too.
func TestAdvance_aRehearsalsApprovedCall_waitsForTheAuthor(t *testing.T) {
	t.Parallel()
	h, start := rehearsalHarness(t, Proposal{Tool: "crm.note", Args: []byte(`{"text":"hi"}`)})
	start.Stage = h.start(t, generousBudget()).Stage
	if _, err := h.runner.Advance(context.Background(), start); err != nil {
		t.Fatalf("Advance (to the Gate's hold): %v", err)
	}
	held := headOf(t, h, start)
	if held.Kind != domain.StepApprovalRequested {
		t.Fatalf("head %s, want the Gate holding the note", held.Kind)
	}
	h.seed(t, start, domain.Step{Kind: domain.StepRehearsalDecided,
		Payload: mustJSON(domain.RehearsalDecidedPayload{AtSeq: held.Seq, Approved: true, By: "usr_ana"})})

	if _, err := h.runner.Advance(context.Background(), start); err != nil {
		t.Fatalf("Advance (approved): %v", err)
	}
	if head := headOf(t, h, start); head.Kind != domain.StepWorldAwaited || len(h.tools.invocations) != 0 {
		t.Errorf("head %s, %d calls; want world_awaited and nothing called", head.Kind, len(h.tools.invocations))
	}
}
