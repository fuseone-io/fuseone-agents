package trigger_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/ledger"
)

/*
A run an event opens inherits its source's work item (#204, slice 3): the
card the source claimed, or failing that what the source itself inherited.
Attribution only, read from the source's own record.
*/

const (
	issue = domain.WorkItemRef("github:fuseone-io/fuseone-agents#204")
	pr    = domain.WorkItemRef("github:fuseone-io/fuseone-agents#240")
)

func claimOn(t *testing.T, store *ledger.Memory, id domain.RunID, agent domain.AgentID, ref domain.WorkItemRef) {
	t.Helper()
	if _, err := store.Append(context.Background(), domain.Step{
		RunID: id, Kind: domain.StepWorkItemClaimed, Scope: chainScope, AgentID: agent, VersionID: "v1",
		At: time.Date(2026, 10, 31, 8, 10, 0, 0, time.UTC), Payload: mustJSON(domain.WorkItemClaimedPayload{Ref: ref}),
	}); err != nil {
		t.Fatalf("claim %s: %v", ref, err)
	}
}

// inheritedBy is the work item sealed on each run the sweep opened.
func inheritedBy(t *testing.T, store *ledger.Memory) map[domain.RunID]*domain.RunWorkItem {
	t.Helper()
	ids, err := store.Runs(context.Background())
	if err != nil {
		t.Fatalf("runs: %v", err)
	}
	out := map[domain.RunID]*domain.RunWorkItem{}
	for _, id := range ids {
		steps, _ := store.Read(context.Background(), id, domain.FirstSeq)
		if strings.HasPrefix(steps[0].IdemKey, "start:") {
			continue
		}
		var p domain.RunStartedPayload
		if err := json.Unmarshal(steps[0].Payload, &p); err != nil {
			t.Fatalf("decode start of %s: %v", id, err)
		}
		out[id] = p.WorkItem
	}
	return out
}

func onlyInherited(t *testing.T, store *ledger.Memory) *domain.RunWorkItem {
	t.Helper()
	got := inheritedBy(t, store)
	if len(got) != 1 {
		t.Fatalf("the sweep opened %d runs, want 1", len(got))
	}
	for _, item := range got {
		return item
	}
	return nil
}

func sweep(t *testing.T, store *ledger.Memory) {
	t.Helper()
	if _, err := chainDispatcher(store).Sweep(context.Background(), 50); err != nil {
		t.Fatalf("Sweep: %v", err)
	}
}

// The source claimed the PR and itself inherited the issue: its own claim
// wins.
func TestDispatcher_listenerInheritsSourceClaim(t *testing.T) {
	t.Parallel()
	store := ledger.NewMemory()
	sealStart(t, store, "run-dev", "agent-a", domain.RunStartedPayload{
		Trigger:  "webhook",
		WorkItem: &domain.RunWorkItem{Ref: issue, Source: domain.WorkItemFromGitHubDelivery},
	})
	claimOn(t, store, "run-dev", "agent-a", pr)
	finishRun(t, store, "run-dev")

	sweep(t, store)

	want := domain.RunWorkItem{Ref: pr, Source: domain.WorkItemFromEvent, FromRun: "run-dev"}
	if got := onlyInherited(t, store); got == nil || *got != want {
		t.Fatalf("inherited %+v, want %+v", got, want)
	}
}

func TestDispatcher_sourceWithoutClaim_passesItsInheritance(t *testing.T) {
	t.Parallel()
	store := ledger.NewMemory()
	sealStart(t, store, "run-review", "agent-a", domain.RunStartedPayload{
		Trigger: domain.TriggerEvent, Chain: &domain.EventChain{Root: "run-dev", Hop: 1},
		WorkItem: &domain.RunWorkItem{Ref: issue, Source: domain.WorkItemFromEvent, FromRun: "run-dev"},
	})
	finishRun(t, store, "run-review")

	sweep(t, store)

	want := domain.RunWorkItem{Ref: issue, Source: domain.WorkItemFromEvent, FromRun: "run-review"}
	if got := onlyInherited(t, store); got == nil || *got != want {
		t.Fatalf("inherited %+v, want %+v", got, want)
	}
}

// A source with neither — or one an older release opened, whose start has no
// such field — opens its listener exactly as before, attributed to nothing.
func TestDispatcher_noWorkItem_opensWithout(t *testing.T) {
	t.Parallel()
	store := ledger.NewMemory()
	startRun(t, store, "run-a", "agent-a", nil)
	finishRun(t, store, "run-a")

	sweep(t, store)

	if got := onlyInherited(t, store); got != nil {
		t.Fatalf("inherited %+v, want nothing", got)
	}
}

// The listener's inheritance never holds the card: while the listener is
// still open, another run of the listener's own agent claims it.
func TestDispatcher_listenerInheritance_neverBlocksAClaim(t *testing.T) {
	t.Parallel()
	store := ledger.NewMemory()
	startRun(t, store, "run-dev", "agent-a", nil)
	claimOn(t, store, "run-dev", "agent-a", issue)
	finishRun(t, store, "run-dev")
	sweep(t, store)
	if got := onlyInherited(t, store); got == nil || got.Ref != issue {
		t.Fatalf("inherited %+v, want %s", got, issue)
	}

	startRun(t, store, "run-b2", "agent-b", nil)
	claimOn(t, store, "run-b2", "agent-b", issue)
}
