package engine

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/gate"
)

/*
$fuseone.work_item.link, as a run makes it (#204, slice 2b).

The engine executes the link itself, like the claim: the run supplies the pull
request, the engine fills the root from the card the run holds and names the
proof from the run's own record. Proven — the run's create_pull_request
answered naming that pull request — it passes the Gate like the claim. Unproven
it parks for a person, and the decision is its proof. Either way no tool layer
is ever asked to run it.
*/

const (
	linkRootCard = "github:fuseone-io/fuseone-agents#204"
	linkPR       = "github:fuseone-io/fuseone-agents#240"
	createPR     = domain.ToolID("github-developer.create_pull_request")
	prAnswer     = `{"id":"9","url":"https://github.com/fuseone-io/fuseone-agents/pull/240"}`
)

func linkProposal(ref string) Proposal {
	return Proposal{Tool: domain.ToolWorkItemLink, Args: []byte(`{"ref":` + mustJSONString(ref) + `}`)}
}

func createPRProposal() Proposal {
	return Proposal{Tool: createPR, Args: []byte(`{"title":"fix","head":"factory/204-fix","base":"main"}`)}
}

// linkHarness opens run-1 for a developer that declares the claim, the link
// and create_pull_request, in copilot, tainted by an untrusted read — the
// shape a factory Developer run has. The pull request server answers prAnswer.
func linkHarness(t *testing.T, proposals ...Proposal) (*harness, Start) {
	t.Helper()
	h := newHarness(t, proposals...)
	h.runner.deps.Catalog = staticCatalog{createPR: domain.EffectWrite, "crm.note": domain.EffectWrite}
	h.tools.body = []byte(prAnswer)
	start := h.start(t, generousBudget())
	start.Pack = gate.NewPack(domain.ToolWorkItemClaim, domain.ToolWorkItemLink, createPR, "crm.note")
	start.Stage = domain.StageCopilot
	labels := domain.ScopeLabels(start.Scope).Union(domain.NewLabels(domain.LabelUntrusted))
	openRunWithInput(t, h, start, "cron", []byte("pick a card"), labels)
	return h, start
}

// approveRequest answers the run's latest approval request, naming it, as the
// console does.
func approveRequest(t *testing.T, h *harness, approved bool) int64 {
	t.Helper()
	requested := h.stepsOf(t, "run-1", domain.StepApprovalRequested)
	if len(requested) == 0 {
		t.Fatalf("no approval to answer in %v", h.kinds(t))
	}
	at := requested[len(requested)-1]
	sealed, err := h.ledger.Append(context.Background(), domain.Step{
		RunID: "run-1", Kind: domain.StepApprovalDecided,
		Scope: at.Scope, AgentID: at.AgentID, VersionID: at.VersionID, At: at.At,
		Payload: mustJSON(domain.ApprovalDecidedPayload{Approved: approved, By: "ana", AtSeq: at.Seq}),
	})
	if err != nil {
		t.Fatalf("decide: %v", err)
	}
	return sealed.Seq
}

func linkedPayloads(t *testing.T, h *harness) []domain.WorkItemLinkedPayload {
	t.Helper()
	var out []domain.WorkItemLinkedPayload
	for _, st := range h.stepsOf(t, "run-1", domain.StepWorkItemLinked) {
		var p domain.WorkItemLinkedPayload
		if err := json.Unmarshal(st.Payload, &p); err != nil {
			t.Fatalf("decode link: %v", err)
		}
		out = append(out, p)
	}
	return out
}

// openPullRequest claims the root and has create_pull_request approved and
// made, leaving the run ready for its next proposal.
func openPullRequest(t *testing.T, h *harness, start Start) {
	t.Helper()
	advance(t, h, start, 2) // the claim; create_pull_request parks
	approveRequest(t, h, true)
	advance(t, h, start, 1) // the approved create_pull_request
}

func TestAdvance_linkAfterTheRunsOwnPullRequest_linksWithoutApprovalAndNeverInvokesTheTool(t *testing.T) {
	t.Parallel()
	h, start := linkHarness(t, claimProposal(linkRootCard), createPRProposal(), linkProposal(linkPR))
	openPullRequest(t, h, start)
	requestsBefore := len(h.stepsOf(t, "run-1", domain.StepApprovalRequested))

	advance(t, h, start, 1)

	if got := len(h.stepsOf(t, "run-1", domain.StepApprovalRequested)); got != requestsBefore {
		t.Fatalf("the proven link asked for approval: %v", h.kinds(t))
	}
	links := linkedPayloads(t, h)
	if len(links) != 1 {
		t.Fatalf("work_item_linked steps = %d in %v, want 1", len(links), h.kinds(t))
	}
	returned := h.stepsOf(t, "run-1", domain.StepToolReturned)
	prReturn := returned[1] // the claim's, then create_pull_request's
	want := domain.WorkItemLinkedPayload{
		Alias: linkPR, Root: linkRootCard,
		Proof: domain.WorkItemLinkProof{Kind: domain.StepToolReturned, Seq: prReturn.Seq},
	}
	if links[0] != want {
		t.Fatalf("link = %+v, want %+v", links[0], want)
	}
	if !slices.Equal(h.tools.invocations, []domain.ToolID{createPR}) {
		t.Fatalf("tool layer invocations = %v; only create_pull_request may reach it", h.tools.invocations)
	}
}

// What the engine records the answer named is what the ledger later judges
// the proof by.
func TestAdvance_createPullRequestReturn_recordsThePullRequestItsAnswerNamed(t *testing.T) {
	t.Parallel()
	h, start := linkHarness(t, claimProposal(linkRootCard), createPRProposal())
	openPullRequest(t, h, start)

	payloads := returnedPayloads(t, h)
	if len(payloads) != 2 || payloads[1].Tool != createPR || payloads[1].PullRequest != linkPR {
		t.Fatalf("returns = %+v; want create_pull_request's to name %s", payloads, linkPR)
	}
	if payloads[0].PullRequest != "" {
		t.Fatalf("the claim's return names a pull request: %+v", payloads[0])
	}
}

func TestAdvance_failedOrOtherReturns_nameNoPullRequest(t *testing.T) {
	t.Parallel()
	h, start := linkHarness(t, claimProposal(linkRootCard), createPRProposal())
	h.tools.failed, h.tools.errorCode = true, "tool_error"
	openPullRequest(t, h, start)

	for _, p := range returnedPayloads(t, h) {
		if p.PullRequest != "" {
			t.Fatalf("a failed create_pull_request named %s: %+v", p.PullRequest, p)
		}
	}
}

// Without the proof, the link is an ordinary write: it parks, and once a
// person approves it the engine links it with that decision as its proof.
func TestAdvance_unprovenLink_parksThenLinksWithTheDecisionAsProof(t *testing.T) {
	t.Parallel()
	h, start := linkHarness(t, claimProposal(linkRootCard), linkProposal(linkPR))

	advance(t, h, start, 2)
	if got := h.stepsOf(t, "run-1", domain.StepApprovalRequested); len(got) != 1 {
		t.Fatalf("the unproven link did not park: %v", h.kinds(t))
	}
	if len(linkedPayloads(t, h)) != 0 {
		t.Fatalf("the unproven link was sealed before anybody approved it")
	}

	decided := approveRequest(t, h, true)
	advance(t, h, start, 1)

	links := linkedPayloads(t, h)
	want := domain.WorkItemLinkProof{Kind: domain.StepApprovalDecided, Seq: decided}
	if len(links) != 1 || links[0].Proof != want || links[0].Root != linkRootCard {
		t.Fatalf("links = %+v; want one to %s proved by the decision at %d", links, linkRootCard, decided)
	}
	if len(h.tools.invocations) != 0 {
		t.Fatalf("the link reached the tool layer: %v", h.tools.invocations)
	}
}

// A create_pull_request whose answer named another pull request proves
// nothing about this one.
func TestAdvance_linkOfAPullRequestTheRunDidNotOpen_parks(t *testing.T) {
	t.Parallel()
	h, start := linkHarness(t, claimProposal(linkRootCard), createPRProposal(),
		linkProposal("github:fuseone-io/fuseone-agents#241"))
	openPullRequest(t, h, start)
	requestsBefore := len(h.stepsOf(t, "run-1", domain.StepApprovalRequested))

	advance(t, h, start, 1)

	if got := len(h.stepsOf(t, "run-1", domain.StepApprovalRequested)); got != requestsBefore+1 {
		t.Fatalf("a link of a pull request the run did not open went through unasked: %v", h.kinds(t))
	}
}

// A link the record already refuses is refused at the Gate, never parked and
// never executed: no card held, or a pull request of another repository.
func TestAdvance_linkThatCannotBeMade_refusedAtTheGate(t *testing.T) {
	t.Parallel()
	// Each refused for its own reason, which is what the model reads back:
	// the reason, not only the verdict, is the part it can act on.
	cases := map[string]struct {
		proposals []Proposal
		reason    string
	}{
		"no card held":       {[]Proposal{linkProposal(linkPR)}, "holds no card"},
		"another repository": {[]Proposal{claimProposal(linkRootCard), linkProposal("github:fuseone-io/fuseone-site#240")}, "same repository"},
		"the card itself":    {[]Proposal{claimProposal(linkRootCard), linkProposal(linkRootCard)}, "same repository"},
		"not a reference":    {[]Proposal{claimProposal(linkRootCard), linkProposal("https://github.com/fuseone-io/fuseone-agents/pull/240")}, "github:owner/repo#123"},
	}
	for name, c := range cases {
		h, start := linkHarness(t, c.proposals...)
		advance(t, h, start, len(c.proposals))

		decisions := h.stepsOf(t, "run-1", domain.StepGateDecided)
		var last domain.GateDecidedPayload
		if err := json.Unmarshal(decisions[len(decisions)-1].Payload, &last); err != nil {
			t.Fatalf("%s: decode: %v", name, err)
		}
		if last.Tool != domain.ToolWorkItemLink || last.Verdict != domain.VerdictBlock || last.Rule != gate.RuleContract ||
			!strings.Contains(last.Reason, c.reason) {
			t.Errorf("%s: link decision = %s %s/%s (%s), want block/contract saying %q",
				name, last.Tool, last.Verdict, last.Rule, last.Reason, c.reason)
		}
		if len(linkedPayloads(t, h)) != 0 || slices.Contains(h.tools.invocations, domain.ToolWorkItemLink) {
			t.Errorf("%s: the refused link was sealed or invoked", name)
		}
	}
}

// The ledger's own refusal — here, a pull request another card already owns —
// is the model's answer, a failed result with its code, and seals nothing.
func TestAdvance_linkOfAPullRequestAnotherCardOwns_answeredLinkedElsewhere(t *testing.T) {
	t.Parallel()
	h, start := linkHarness(t, claimProposal(linkRootCard), createPRProposal(), linkProposal(linkPR))
	sealOtherCardsLink(t, h)
	openPullRequest(t, h, start)

	advance(t, h, start, 1)

	if len(linkedPayloads(t, h)) != 0 {
		t.Fatalf("a link of another card's pull request was sealed: %v", h.kinds(t))
	}
	payloads := returnedPayloads(t, h)
	last := payloads[len(payloads)-1]
	if last.Tool != domain.ToolWorkItemLink || !last.Failed || last.ErrorCode != CodeWorkItemLinkedElsewhere {
		t.Fatalf("link result = %+v, want a failure coded %s", last, CodeWorkItemLinkedElsewhere)
	}
}

// sealOtherCardsLink has run-0, of another agent, link linkPR to #7 first.
func sealOtherCardsLink(t *testing.T, h *harness) {
	t.Helper()
	ctx := context.Background()
	scope := domain.Scope{Company: "acme", Area: "cx"}
	for _, st := range []domain.Step{
		{Kind: domain.StepRunStarted},
		{Kind: domain.StepWorkItemClaimed, Payload: mustJSON(domain.WorkItemClaimedPayload{Ref: "github:fuseone-io/fuseone-agents#7"})},
		{Kind: domain.StepToolCalled, Payload: mustJSON(domain.ToolCalledPayload{Tool: createPR, Effect: domain.EffectWrite})},
		{Kind: domain.StepToolReturned, Payload: mustJSON(domain.ToolReturnedPayload{Tool: createPR, PullRequest: linkPR})},
		{Kind: domain.StepWorkItemLinked, Payload: mustJSON(domain.WorkItemLinkedPayload{
			Alias: linkPR, Root: "github:fuseone-io/fuseone-agents#7",
			Proof: domain.WorkItemLinkProof{Kind: domain.StepToolReturned, Seq: 4},
		})},
	} {
		st.RunID, st.Scope, st.AgentID, st.VersionID = "run-0", scope, "developer-2", "v1"
		if _, err := h.ledger.Append(ctx, st); err != nil {
			t.Fatalf("seal run-0's %s: %v", st.Kind, err)
		}
	}
}

// Declared once, the link is ambient like the claim: reachable at whatever
// step the run has got to.
func TestEnvelope_declaredLink_isReachableAtEveryStep(t *testing.T) {
	t.Parallel()
	start := Start{
		Pack: gate.NewPack(domain.ToolWorkItemLink, "a.one", "a.two"),
		Steps: []Envelope{
			{Name: "first", Reaches: []domain.ToolID{"a.one"}},
			{Name: "second", Reaches: []domain.ToolID{"a.two"}},
		},
	}
	state := State{Called: []domain.ToolID{"a.two"}}
	if !envelopeForState(start, state).Allows(domain.ToolWorkItemLink) {
		t.Fatal("the declared link is unreachable at the second step")
	}
	start.Pack = gate.NewPack("a.one", "a.two")
	if envelopeForState(start, state).Allows(domain.ToolWorkItemLink) {
		t.Fatal("an agent that never declared the link is offered it")
	}
}
