package engine

import (
	"context"
	"encoding/json"
	"slices"
	"testing"

	effectdedupe "github.com/fuseone/agents/internal/dedupe"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/gate"
)

/*
$fuseone.work_item.claim, as a run makes it (#206).

The engine executes the claim itself: it records a work_item_claimed step the
ledger judges as it seals it, and answers the model from that judgement. The
tool layer never sees it, it is never answered from the run's own record, and
it passes the Gate as a coordination operation.
*/

const claimedCard = "github:fuseone-io/fuseone-agents#123"

func claimProposal(ref string) Proposal {
	return Proposal{Tool: domain.ToolWorkItemClaim, Args: []byte(`{"ref":` + mustJSONString(ref) + `}`)}
}

// claimHarness opens run-1 for an agent that declares the claim, in copilot,
// with the run already tainted by an untrusted read.
func claimHarness(t *testing.T, proposals ...Proposal) (*harness, Start) {
	t.Helper()
	h := newHarness(t, proposals...)
	start := h.start(t, generousBudget())
	start.Pack = gate.NewPack(domain.ToolWorkItemClaim, "crm.lookup", "crm.note")
	start.Stage = domain.StageCopilot
	labels := domain.ScopeLabels(start.Scope).Union(domain.NewLabels(domain.LabelUntrusted))
	openRunWithInput(t, h, start, "cron", []byte("pick a card"), labels)
	return h, start
}

func (h *harness) stepsOf(t *testing.T, runID domain.RunID, kind domain.StepKind) []domain.Step {
	t.Helper()
	steps, err := h.ledger.Read(context.Background(), runID, domain.FirstSeq)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	var out []domain.Step
	for _, st := range steps {
		if st.Kind == kind {
			out = append(out, st)
		}
	}
	return out
}

func returnedPayloads(t *testing.T, h *harness) []domain.ToolReturnedPayload {
	t.Helper()
	var out []domain.ToolReturnedPayload
	for _, st := range h.stepsOf(t, "run-1", domain.StepToolReturned) {
		var p domain.ToolReturnedPayload
		if err := json.Unmarshal(st.Payload, &p); err != nil {
			t.Fatalf("decode tool_returned: %v", err)
		}
		out = append(out, p)
	}
	return out
}

func advance(t *testing.T, h *harness, start Start, times int) {
	t.Helper()
	for range times {
		if _, err := h.runner.Advance(context.Background(), start); err != nil {
			t.Fatalf("Advance: %v", err)
		}
	}
}

// After an untrusted read, the claim goes through with no approval asked — and
// an ordinary write in the same run still parks for one.
func TestAdvance_workItemClaimInATaintedRun_claimsWithoutApprovalWhileAWriteStillParks(t *testing.T) {
	t.Parallel()
	h, start := claimHarness(t,
		claimProposal(claimedCard),
		Proposal{Tool: "crm.note", Args: []byte(`{"id":"42","note":"on it"}`)},
	)

	advance(t, h, start, 1)
	if got := h.stepsOf(t, "run-1", domain.StepApprovalRequested); len(got) != 0 {
		t.Fatalf("the claim asked for approval: %d approval_requested steps", len(got))
	}
	claimed := h.stepsOf(t, "run-1", domain.StepWorkItemClaimed)
	if len(claimed) != 1 {
		t.Fatalf("work_item_claimed steps = %d, want 1 in %v", len(claimed), h.kinds(t))
	}
	var p domain.WorkItemClaimedPayload
	if err := json.Unmarshal(claimed[0].Payload, &p); err != nil || p.Ref != claimedCard {
		t.Fatalf("claimed payload = %+v (%v), want ref %q", p, err, claimedCard)
	}
	if len(h.tools.invocations) != 0 {
		t.Fatalf("the claim reached the tool layer: %v", h.tools.invocations)
	}

	advance(t, h, start, 1)
	if got := h.stepsOf(t, "run-1", domain.StepApprovalRequested); len(got) != 1 {
		t.Fatalf("the tainted write in the same run did not park: %v", h.kinds(t))
	}
}

// The second claim is executed against the ledger again, not answered from
// the first: a fresh call, allowed rather than skipped as a duplicate, sealed
// as a second claim step.
func TestAdvance_workItemClaimTwiceInOneRun_secondIsJudgedAgainNotCached(t *testing.T) {
	t.Parallel()
	h, start := claimHarness(t, claimProposal(claimedCard), claimProposal(claimedCard))

	advance(t, h, start, 2)

	if got := h.stepsOf(t, "run-1", domain.StepWorkItemClaimed); len(got) != 2 {
		t.Fatalf("work_item_claimed steps = %d, want 2 (the retry judged again) in %v", len(got), h.kinds(t))
	}
	for _, st := range h.stepsOf(t, "run-1", domain.StepGateDecided) {
		var d domain.GateDecidedPayload
		if err := json.Unmarshal(st.Payload, &d); err != nil {
			t.Fatalf("decode gate_decided: %v", err)
		}
		if d.Verdict != domain.VerdictAllow || d.Effect != domain.EffectWrite {
			t.Fatalf("claim decision = %s effect %s, want allow as a write", d.Verdict, d.Effect)
		}
	}
	called := h.stepsOf(t, "run-1", domain.StepToolCalled)
	if len(called) != 2 || called[0].IdemKey == called[1].IdemKey {
		t.Fatalf("tool_called = %d with keys %v; each claim is its own call", len(called), idemKeys(called))
	}
	for _, p := range returnedPayloads(t, h) {
		if p.Failed || p.Cached {
			t.Fatalf("claim result = %+v, want a fresh success", p)
		}
	}
}

func TestAdvance_workItemClaimOfASecondCard_refusedToTheModel(t *testing.T) {
	t.Parallel()
	h, start := claimHarness(t, claimProposal(claimedCard), claimProposal("github:fuseone-io/fuseone-agents#124"))

	advance(t, h, start, 2)

	if got := h.stepsOf(t, "run-1", domain.StepWorkItemClaimed); len(got) != 1 {
		t.Fatalf("work_item_claimed steps = %d, want only the first card", len(got))
	}
	results := returnedPayloads(t, h)
	if len(results) != 2 || !results[1].Failed || results[1].ErrorCode != CodeWorkItemRunHoldsAnother {
		t.Fatalf("second claim result = %+v, want failed with %s", results, CodeWorkItemRunHoldsAnother)
	}
}

func TestAdvance_workItemClaimHeldByAnotherOpenRun_refusedToTheModel(t *testing.T) {
	t.Parallel()
	h, start := claimHarness(t, claimProposal(claimedCard))
	holder := start
	holder.RunID = "run-0"
	openRunWithInput(t, h, holder, "cron", []byte("pick a card"), nil)
	if _, err := h.ledger.Append(context.Background(), domain.Step{
		RunID: "run-0", Kind: domain.StepWorkItemClaimed, Scope: start.Scope, AgentID: start.AgentID,
		Payload: mustJSON(domain.WorkItemClaimedPayload{Ref: claimedCard}),
	}); err != nil {
		t.Fatalf("the holder's claim: %v", err)
	}

	advance(t, h, start, 1)

	if got := h.stepsOf(t, "run-1", domain.StepWorkItemClaimed); len(got) != 0 {
		t.Fatalf("run-1 sealed a claim on a held card")
	}
	results := returnedPayloads(t, h)
	if len(results) != 1 || !results[0].Failed || results[0].ErrorCode != CodeWorkItemHeld {
		t.Fatalf("claim result = %+v, want failed with %s", results, CodeWorkItemHeld)
	}
}

func TestAdvance_workItemClaimWithNoUsableRef_refusedToTheModel(t *testing.T) {
	t.Parallel()
	h, start := claimHarness(t, Proposal{Tool: domain.ToolWorkItemClaim, Args: []byte(`{"ref":"  "}`)})

	advance(t, h, start, 1)

	results := returnedPayloads(t, h)
	if len(results) != 1 || !results[0].Failed || results[0].ErrorCode != CodeWorkItemRefInvalid {
		t.Fatalf("claim result = %+v, want failed with %s", results, CodeWorkItemRefInvalid)
	}
}

// Platform-owned: the claim never moves a run to a later step, even where an
// author anchored it in one, and it is offered whatever step the run is at.
func TestStepAt_workItemClaim_neverAdvancesAStep(t *testing.T) {
	t.Parallel()
	start := Start{Steps: []Envelope{
		{Name: "pick", Reaches: []domain.ToolID{"crm.lookup"}},
		{Name: "act", Reaches: []domain.ToolID{"crm.note", domain.ToolWorkItemClaim}},
	}}
	if at := StepAt(start, []domain.ToolID{domain.ToolWorkItemClaim}); at != 0 {
		t.Fatalf("StepAt after a claim = %d, want 0", at)
	}
}

func TestEnvelope_workItemClaim_offeredOnlyWhenDeclared(t *testing.T) {
	t.Parallel()
	steps := []Envelope{
		{Name: "pick", Reaches: []domain.ToolID{"crm.lookup"}},
		{Name: "act", Reaches: []domain.ToolID{"crm.note"}},
	}
	declared := Start{Pack: gate.NewPack(domain.ToolWorkItemClaim, "crm.lookup", "crm.note"), Steps: steps}
	state := State{Called: []domain.ToolID{"crm.note"}}
	if !envelopeForState(declared, state).Allows(domain.ToolWorkItemClaim) {
		t.Fatal("an agent that declares the claim is not offered it at a later step")
	}
	undeclared := Start{Pack: gate.NewPack("crm.lookup", "crm.note"), Steps: steps}
	if envelopeForState(undeclared, state).Allows(domain.ToolWorkItemClaim) {
		t.Fatal("an agent that never declared the claim is offered it")
	}
}

// Outside the MCP availability checks: no server serves it, so a "$fuseone"
// server that nobody connected can neither park the run nor hide the tool.
func TestPlan_workItemClaim_neverAppearsUnavailable(t *testing.T) {
	t.Parallel()
	h, start := claimHarness(t)
	planner := &capturingPlanner{}
	h.runner.deps.Planner = planner
	h.runner.deps.Schemas = describing{"crm.lookup": true, "crm.note": true}
	h.runner.deps.Connections = crmHeld(true)
	h.runner.deps.Servers = &expecting{servers: map[string]bool{"$fuseone": true}}

	if _, err := h.runner.Advance(context.Background(), start); err != nil {
		t.Fatalf("Advance: %v", err)
	}
	if len(planner.inputs) != 1 {
		t.Fatalf("planner called %d times, want 1", len(planner.inputs))
	}
	in := planner.inputs[0]
	if slices.Contains(in.Unavailable, domain.ToolWorkItemClaim) {
		t.Fatalf("the claim was reported unavailable: %v", in.Unavailable)
	}
	if !slices.Contains(in.Tools, domain.ToolWorkItemClaim) {
		t.Fatalf("the claim was not offered: %v", in.Tools)
	}
}

func idemKeys(steps []domain.Step) []string {
	out := make([]string, len(steps))
	for i, st := range steps {
		out[i] = st.IdemKey
	}
	return out
}

// A catalogue that declared semantic dedupe for the claim would let an earlier
// run's record answer it. The engine never asks.
func TestAdvance_workItemClaim_neverConsultsSemanticDedupe(t *testing.T) {
	t.Parallel()
	h, start := claimHarness(t, claimProposal(claimedCard))
	store := &fakeDedupeStore{
		lookupFound:   []bool{true},
		lookupRecords: []effectdedupe.Record{{State: effectdedupe.StateConfirmed, RunID: "run-9", Seq: 4}},
	}
	enableDedupe(h, store)
	h.runner.deps.Catalog = dedupeCatalog{
		staticCatalog: staticCatalog{},
		dedupes: map[domain.ToolID]domain.ToolDedupe{
			domain.ToolWorkItemClaim: {WindowSeconds: 3600, ArgPaths: []string{"ref"}},
		},
	}

	advance(t, h, start, 1)

	if got := h.stepsOf(t, "run-1", domain.StepWorkItemClaimed); len(got) != 1 {
		t.Fatalf("work_item_claimed steps = %d, want 1 in %v", len(got), h.kinds(t))
	}
	if len(store.lookups) != 0 || len(store.reserves) != 0 {
		t.Fatalf("semantic dedupe consulted for the claim: %d lookups, %d reserves",
			len(store.lookups), len(store.reserves))
	}
}
