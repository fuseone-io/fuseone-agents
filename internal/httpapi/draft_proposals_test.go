package httpapi

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/authoring/blocks"
	"github.com/fuseone/agents/internal/authoring/draft"
	"github.com/fuseone/agents/internal/authoring/drafts"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/spec"
)

/*
The proposals' routes (NT-018 phase 3, contract 3). The store decides who may
accept what and against which ETag; these pin what the route decides: that
acceptance carries only the id, how each refusal answers, and how a proposal
reads on the wire.
*/

func (d *draftStub) Proposals(_ context.Context, _ domain.AgentID, c drafts.Caller) ([]drafts.Proposal, error) {
	d.callers = append(d.callers, c)
	return d.proposals, d.err
}

func (d *draftStub) Accept(_ context.Context, a drafts.Acceptance) (drafts.Opened, error) {
	d.accepted = append(d.accepted, a)
	return d.opened, d.err
}

func (d *draftStub) Reject(_ context.Context, _ domain.AgentID, id string, c drafts.Caller) error {
	d.rejected = append(d.rejected, id)
	d.callers = append(d.callers, c)
	return d.err
}

func TestAcceptDraftProposal_carriesOnlyTheIdAndTheCallersAuthority(t *testing.T) {
	t.Parallel()
	stub := &draftStub{opened: someDraft()}
	resp, err := draftServer(stub).AcceptDraftProposal(inArea("cx", domain.RoleAuthor),
		openapi.AcceptDraftProposalRequestObject{AgentId: "triage", ProposalId: "p-1"})
	if err != nil {
		t.Fatalf("AcceptDraftProposal: %v", err)
	}
	got, ok := resp.(openapi.AcceptDraftProposal200JSONResponse)
	if !ok || got.Headers.ETag != `"v1-draft"` {
		t.Fatalf("response = %T, want 200 with the draft's ETag", resp)
	}
	a := stub.accepted[0]
	if a.Proposal != "p-1" || a.Agent != "triage" || a.Caller.By != "usr_ana" {
		t.Errorf("acceptance = %+v", a)
	}
	if !a.Caller.Allow(domain.Scope{Company: "acme", Area: "cx"}) || a.Caller.Allow(domain.Scope{Company: "acme", Area: "ops"}) {
		t.Error("Allow does not answer agent:publish as the caller holds it")
	}
}

func TestAcceptDraftProposal_refusals_answerAsTheContractSays(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		err  error
		want string
	}{
		"not the caller's":  {drafts.ErrNotFound, "404"},
		"decided already":   {drafts.ErrDecided, "409"},
		"invalid when made": {drafts.ErrProposalInvalid, "400"},
		"refused now":       {draft.ErrRefused, "400"},
		"a destination":     {&drafts.ForbiddenError{Scope: domain.Scope{Company: "acme", Area: "ops"}}, "403"},
		"an older draft":    {&domain.StaleError{Current: `"v1-now"`}, "412"},
	} {
		stub := &draftStub{err: tc.err}
		resp, err := draftServer(stub).AcceptDraftProposal(inArea("cx", domain.RoleAuthor),
			openapi.AcceptDraftProposalRequestObject{AgentId: "triage", ProposalId: "p-1"})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got := acceptStatus(t, resp); got != tc.want {
			t.Errorf("%s: answered %T, want %s", name, resp, tc.want)
		}
	}
}

func acceptStatus(t *testing.T, resp openapi.AcceptDraftProposalResponseObject) string {
	t.Helper()
	switch r := resp.(type) {
	case openapi.AcceptDraftProposal404ApplicationProblemPlusJSONResponse:
		return "404"
	case openapi.AcceptDraftProposal409ApplicationProblemPlusJSONResponse:
		return "409"
	case openapi.AcceptDraftProposal400ApplicationProblemPlusJSONResponse:
		return "400"
	case openapi.AcceptDraftProposal403ApplicationProblemPlusJSONResponse:
		return "403"
	case openapi.AcceptDraftProposal412ApplicationProblemPlusJSONResponse:
		if r.Headers.ETag == nil || *r.Headers.ETag != `"v1-now"` {
			t.Errorf("412 ETag = %v, want the current one", r.Headers.ETag)
		}
		return "412"
	}
	return "other"
}

func TestRejectDraftProposal_answersAsTheContractSays(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		err  error
		want any
	}{
		"rejected":         {nil, openapi.RejectDraftProposal204Response{}},
		"not the caller's": {drafts.ErrNotFound, openapi.RejectDraftProposal404ApplicationProblemPlusJSONResponse{}},
		"decided already":  {drafts.ErrDecided, openapi.RejectDraftProposal409ApplicationProblemPlusJSONResponse{}},
	} {
		stub := &draftStub{err: tc.err}
		resp, err := draftServer(stub).RejectDraftProposal(inArea("cx", domain.RoleAuthor),
			openapi.RejectDraftProposalRequestObject{AgentId: "triage", ProposalId: "p-1"})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if typeName(resp) != typeName(tc.want) {
			t.Errorf("%s: answered %T, want %T", name, resp, tc.want)
		}
	}
}

func typeName(v any) string { return fmtType(v) }

// A proposal reads on the wire as the operation it proposes, block
// operations included, with whether it is valid and how it was decided.
func TestListDraftProposals_showsEachProposalsOperationAndState(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)
	stub := &draftStub{proposals: []drafts.Proposal{
		{ID: "p-1", Read: `"r1"`, At: at, Valid: true, Decision: "accepted",
			Op: draft.InsertBlock{At: 1, Type: blocks.Never, Text: "Prometer prazo.", Locale: "pt-BR"}},
		{ID: "p-2", Read: `"r1"`, At: at, Valid: false, Refusal: "only a person may apply this operation",
			Op: draft.SetIdentity{Name: "T", Scope: domain.Scope{Company: "acme", Area: "ops"}}},
		{ID: "p-3", Read: `"r1"`, At: at, Valid: true},
	}}
	resp, err := draftServer(stub).ListDraftProposals(inArea("cx", domain.RoleAuthor),
		openapi.ListDraftProposalsRequestObject{AgentId: "triage"})
	if err != nil {
		t.Fatalf("ListDraftProposals: %v", err)
	}
	items := resp.(openapi.ListDraftProposals200JSONResponse).Items
	if len(items) != 3 {
		t.Fatalf("items = %d", len(items))
	}
	first := items[0]
	if first.Kind != "insert_block" || first.Operation == nil || *first.Operation.BlockKind != "never" ||
		*first.Operation.At != 1 || *first.Operation.Text != "Prometer prazo." || *first.Decision != "accepted" {
		t.Errorf("block proposal = %+v / %+v", first, first.Operation)
	}
	if items[1].Valid || items[1].Refusal == nil || *items[1].Operation.Area != "ops" {
		t.Errorf("invalid proposal = %+v", items[1])
	}
	if items[2].Operation != nil || items[2].Kind != "" {
		t.Errorf("a proposal whose body was erased shows %+v", items[2])
	}
}

var _ = errors.New

func fmtType(v any) string { return fmt.Sprintf("%T", v) }

// What a proposal shows is what accepting it applies: every non-block kind
// read back through the route's own reader is the same operation, and a
// pointer reads as its value.
func TestOperationView_isTheInverseOfTheRoutesReader(t *testing.T) {
	t.Parallel()
	ops := []draft.Operation{
		draft.SetIdentity{Name: "T", Scope: domain.Scope{Company: "acme", Area: "ops"}},
		&draft.SetIdentity{Name: "P", Scope: domain.Scope{Company: "acme", Area: "cx"}},
		draft.SetStandingApprovals{Policy: domain.StandingForbidden},
		draft.SetModel{Provider: "openai", Model: "m", Effort: "high"},
		draft.SetFallback{Pair: domain.ModelPair{Provider: "p", Model: "m"}},
		draft.SetCachePrefixTTL{TTL: "1h"}, draft.SetReplyLocale{Locale: "en-US"},
		draft.AddTool{Tool: "crm.lookup"}, draft.RemoveTool{Tool: "crm.reply"},
		draft.SetBudget{Budget: domain.Budget{Micros: 1, Tokens: 2, ToolCalls: 3, Steps: 4, WallClockMS: 5}},
		draft.SetTriggers{Triggers: []spec.Trigger{{Type: "webhook", Path: "gh", From: "github", Events: []string{"issues"}}}},
		draft.SetMemoryLearning{Policy: domain.MemoryLearningPolicy{Mode: domain.MemoryLearningOff}},
		draft.SetApprovalDelivery{Policy: domain.ApprovalPolicy{Direct: true, Notify: []domain.UserID{"usr_bia"}}},
		draft.SetEmits{Emits: spec.Emits{{Event: "ticket.triaged", Context: "c", Artifacts: []string{"summary"}}}},
		draft.AddStep{Step: spec.Step{Name: "Ler", Reaches: []domain.ToolID{"crm.lookup"}, StopsWhen: "achar"}},
		draft.EditStep{Name: "Ler", Step: spec.Step{Name: "Ler bem"}}, draft.RemoveStep{Name: "Ler"},
	}
	for _, op := range ops {
		back, err := operationOf(operationView(op))
		if err != nil {
			t.Fatalf("%s: %v", op.Kind(), err)
		}
		want := op
		if p, ok := op.(*draft.SetIdentity); ok {
			want = *p
		}
		if draft.Digest(back) != draft.Digest(want) {
			t.Errorf("%s read back as %#v", op.Kind(), back)
		}
	}
}

// Accepting validates again with the installation's own checks.
func TestAcceptDraftProposal_validatesWithTheInstallationsChecks(t *testing.T) {
	t.Parallel()
	stub := &draftStub{opened: someDraft()}
	server := draftServer(stub).WithAdministration(nil, publishFlowTools(), nil)
	if _, err := server.AcceptDraftProposal(inArea("cx", domain.RoleAuthor),
		openapi.AcceptDraftProposalRequestObject{AgentId: "triage", ProposalId: "p-1"}); err != nil {
		t.Fatalf("AcceptDraftProposal: %v", err)
	}
	a := stub.accepted[0]
	if _, err := a.Validator.Validate(context.Background(), someDraft().Draft, draft.AddTool{Tool: "crm.lokup"},
		draft.FromProposal, a.Now); !errors.Is(err, draft.ErrRefused) {
		t.Fatalf("the route's validator = %v, want the tool refused", err)
	}
}

// A block proposal carries the instructions accepting it now would leave,
// rendered by the codec accepting uses: a new block under its locale's label
// at its position, a rewritten one under the label of the operation's locale.
// Only while it could be accepted: waiting, valid, and on a draft the store
// says it still waits on, the one the model read or one only its own turn's
// acceptances moved (#412).
func TestListDraftProposals_aBlockProposalCarriesTheInstructionsItWouldLeave(t *testing.T) {
	t.Parallel()
	opened := someDraft()
	first := opened.Draft.Blocks[0].ID
	stub := &draftStub{opened: opened, proposals: []drafts.Proposal{
		{ID: "insert", Read: opened.Revision, Current: true, Valid: true, Op: draft.InsertBlock{At: 1, Type: blocks.Never, Text: "Prometer prazo.", Locale: "pt-BR"}},
		{ID: "retitle", Read: opened.Revision, Current: true, Valid: true, Op: draft.ReplaceBlock{ID: first, Text: "Triage.", Locale: "en-US"}},
		{ID: "sibling", Read: `"older"`, Current: true, Valid: true, Op: draft.InsertBlock{At: 1, Type: blocks.Never, Text: "Prometer prazo.", Locale: "pt-BR"}},
		{ID: "stale", Read: `"older"`, Valid: true, Op: draft.InsertBlock{At: 0, Type: blocks.Never, Text: "x", Locale: "pt-BR"}},
		{ID: "decided", Read: opened.Revision, Valid: true, Decision: "rejected", Op: draft.RemoveBlock{ID: first}},
		{ID: "field", Read: opened.Revision, Current: true, Valid: true, Op: draft.AddTool{Tool: "crm.lookup"}},
	}}
	resp, err := draftServer(stub).ListDraftProposals(inArea("cx", domain.RoleAuthor), openapi.ListDraftProposalsRequestObject{AgentId: "triage"})
	if err != nil {
		t.Fatalf("ListDraftProposals: %v", err)
	}
	after := map[string]string{}
	for _, p := range resp.(openapi.ListDraftProposals200JSONResponse).Items {
		after[p.ProposalId] = valueOr(p.InstructionsAfter)
	}
	if got := after["insert"]; !strings.Contains(got, "Objetivo\nTriar.") || !strings.HasSuffix(got, "Nunca\nPrometer prazo.") {
		t.Errorf("insert leaves %q, want the never block under its label, after the objective", got)
	}
	if got := after["sibling"]; !strings.HasSuffix(got, "Nunca\nPrometer prazo.") {
		t.Errorf("a sibling of an accepted proposal leaves %q, want its effect on the draft as it is", got)
	}
	if got := after["retitle"]; !strings.HasPrefix(got, "Purpose\nTriage.") {
		t.Errorf("retitle leaves %q, want the block under the operation's locale's label", got)
	}
	for _, id := range []string{"stale", "decided", "field"} {
		if after[id] != "" {
			t.Errorf("%s carries instructions %q, want none", id, after[id])
		}
	}
}

// A tool proposal carries the effect the catalogue rules for the tool in the
// draft's stored scope, the one accepting is judged in, never a scope the
// console has not saved: unknown for a tool nobody ruled on there, and
// nothing once decided or when the catalogue cannot be read.
func TestListDraftProposals_aToolProposalCarriesItsRuledEffect(t *testing.T) {
	t.Parallel()
	opened := someDraft()
	stub := &draftStub{opened: opened, proposals: []drafts.Proposal{
		{ID: "ruled", Read: opened.Revision, Valid: true, Op: draft.AddTool{Tool: "pay.refund"}},
		{ID: "unruled", Read: opened.Revision, Valid: true, Op: draft.AddTool{Tool: "crm.unknown"}},
		{ID: "decided", Read: opened.Revision, Valid: true, Decision: "accepted", Op: draft.AddTool{Tool: "pay.refund"}},
	}}
	catalogue := &fakeTools{entries: []domain.ToolEntry{
		{ID: "pay.refund", Effect: domain.EffectFinancial, OnSurface: true, Scope: domain.Scope{Company: "acme"}},
	}}
	effects := func(tools Tools) map[string]string {
		resp, err := draftServer(stub).WithAdministration(nil, tools, nil).ListDraftProposals(inArea("cx", domain.RoleAuthor),
			openapi.ListDraftProposalsRequestObject{AgentId: "triage"})
		if err != nil {
			t.Fatalf("ListDraftProposals: %v", err)
		}
		out := map[string]string{}
		for _, p := range resp.(openapi.ListDraftProposals200JSONResponse).Items {
			out[p.ProposalId] = string(valueOr(p.ToolEffect))
		}
		return out
	}
	if got := effects(catalogue); got["ruled"] != "financial" || got["unruled"] != "unknown" || got["decided"] != "" {
		t.Errorf("effects = %v", got)
	}
	if got := effects(&fakeTools{err: errors.New("catalogue down")}); got["ruled"] != "" {
		t.Errorf("an unreadable catalogue still says %q", got["ruled"])
	}
}
