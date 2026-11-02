package engine

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
)

/*
A work_item_linked step, as this release reads it (#204, slice 2a).

A newer worker writes one (2b), and during a rolling upgrade the next turn of
that run can land on a worker on the release before it: it folds the run from
the ledger, and a kind its fold does not know fails the run outright. That is
why the readers shipped first, and these tests keep them reading.
*/

const linkedPR = "github:fuseone-io/fuseone-agents#240"

// linkStep is a link as the writer seals it: the run's identity, the claimed
// card as its root, and a returned call as the proof.
func linkStep(start Start, root string, proofSeq int64) domain.Step {
	return domain.Step{
		RunID: start.RunID, Kind: domain.StepWorkItemLinked, Scope: start.Scope,
		AgentID: start.AgentID, VersionID: start.VersionID, OnBehalfOf: start.OnBehalfOf,
		Payload: mustJSON(domain.WorkItemLinkedPayload{
			Alias: linkedPR, Root: domain.WorkItemRef(root),
			Proof: domain.WorkItemLinkProof{Kind: domain.StepToolReturned, Seq: proofSeq},
		}),
	}
}

// A link changes nothing the engine decides from: not the phase, not what is
// pending, not the grant, not the budget it holds. Only the bookkeeping every
// step moves — its place, and that it was one more step.
func TestApply_workItemLinked_foldsWithNoDecisionChanged(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	start := Start{RunID: "run-1", Scope: domain.Scope{Company: "acme", Area: "cx"}, AgentID: "developer"}
	steps := []domain.Step{
		{RunID: "run-1", Seq: 1, Kind: domain.StepRunStarted, At: at,
			Payload: mustJSON(domain.RunStartedPayload{Trigger: "cron"})},
		{RunID: "run-1", Seq: 2, Kind: domain.StepWorkItemClaimed, At: at,
			Payload: mustJSON(domain.WorkItemClaimedPayload{Ref: claimedCard})},
	}
	before, err := Fold(steps)
	if err != nil {
		t.Fatalf("Fold: %v", err)
	}

	link := linkStep(start, claimedCard, 2)
	link.Seq, link.At = 3, at
	after := before
	if err := after.Apply(link); err != nil {
		t.Fatalf("Apply(work_item_linked) = %v; a worker on this release would fail the run", err)
	}

	want := before
	want.Seq, want.Head, want.Spent.Steps = 3, domain.StepRef{Seq: 3, Kind: domain.StepWorkItemLinked}, before.Spent.Steps+1
	if !reflect.DeepEqual(after, want) {
		t.Fatalf("a link moved the state:\n got %+v\nwant %+v", after, want)
	}
}

// A link that is not even JSON is a corrupt record, refused like any other.
func TestApply_workItemLinkedUndecodable_refused(t *testing.T) {
	t.Parallel()
	s := State{}
	if err := s.Apply(domain.Step{RunID: "run-1", Seq: 1, Kind: domain.StepRunStarted}); err != nil {
		t.Fatalf("Apply(run_started): %v", err)
	}
	broken := domain.Step{RunID: "run-1", Seq: 2, Kind: domain.StepWorkItemLinked, Payload: []byte(`{"alias":`)}
	if err := s.Apply(broken); err == nil {
		t.Fatal("Apply(undecodable link) = nil, want the decode error")
	}
}

// The rolling-upgrade accuser. A run claimed its card, and a newer worker
// sealed a link into it; this release takes the next turn. It must fold the
// run with the link in it and carry on with the work, not fail it.
func TestAdvance_runWithAWorkItemLinkFromANewerWorker_foldsAndResumes(t *testing.T) {
	t.Parallel()
	h, start := claimHarness(t,
		claimProposal(claimedCard),
		Proposal{Tool: "crm.lookup", Args: []byte(`{"id":"42"}`)},
	)
	advance(t, h, start, 1)
	returned := h.stepsOf(t, "run-1", domain.StepToolReturned)
	if len(returned) != 1 {
		t.Fatalf("the claim did not return: %v", h.kinds(t))
	}
	// The newer worker's run opened its pull request, and the link names that
	// return as its proof: the only proof a link without a person has.
	proof := sealPullRequestReturn(t, h, start)
	linked, err := h.ledger.Append(context.Background(), linkStep(start, claimedCard, proof))
	if err != nil {
		t.Fatalf("seal the newer worker's link: %v", err)
	}

	status, err := h.runner.Advance(context.Background(), start)
	if err != nil {
		t.Fatalf("Advance after a link = %v; the run would fail on this release", err)
	}
	if status.Done || status.Phase == PhaseFailed || status.Phase == PhaseParked {
		t.Fatalf("Advance after a link = %+v, want the run still working", status)
	}
	var calledAfter []domain.ToolID
	for _, st := range h.stepsOf(t, "run-1", domain.StepToolCalled) {
		if st.Seq > linked.Seq {
			var p domain.ToolCalledPayload
			if err := decode(st, &p); err != nil {
				t.Fatalf("decode tool_called: %v", err)
			}
			calledAfter = append(calledAfter, p.Tool)
		}
	}
	if !reflect.DeepEqual(calledAfter, []domain.ToolID{"crm.lookup"}) {
		t.Fatalf("calls after the link = %v, want the next proposal carried out; trail %v", calledAfter, h.kinds(t))
	}
}

// sealPullRequestReturn records a create_pull_request of the run that answered
// naming linkedPR, as a newer worker writes it, and returns the return's seq.
func sealPullRequestReturn(t *testing.T, h *harness, start Start) int64 {
	t.Helper()
	var seq int64
	for _, st := range []domain.Step{
		{Kind: domain.StepToolCalled, Payload: mustJSON(domain.ToolCalledPayload{
			Tool: "github.create_pull_request", Effect: domain.EffectWrite})},
		{Kind: domain.StepToolReturned, Payload: mustJSON(domain.ToolReturnedPayload{
			Tool:        "github.create_pull_request",
			PullRequest: &domain.PullRequestStamp{Ref: linkedPR, Via: domain.ProvenanceHostedGitHubApp}})},
	} {
		st.RunID, st.Scope, st.AgentID, st.VersionID = start.RunID, start.Scope, start.AgentID, start.VersionID
		sealed, err := h.ledger.Append(context.Background(), st)
		if err != nil {
			t.Fatalf("seal %s: %v", st.Kind, err)
		}
		seq = sealed.Seq
	}
	return seq
}
