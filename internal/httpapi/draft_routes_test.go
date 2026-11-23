package httpapi

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/authoring/blocks"
	"github.com/fuseone/agents/internal/authoring/draft"
	"github.com/fuseone/agents/internal/authoring/drafts"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
)

/*
The draft's routes (NT-018 1b, part 3).

The store judges who may reach a draft and every precondition, under its
locks, and its own tests and the route's journey cover that against Postgres.
These pin what the route itself decides: the headers it requires, the block
operations it refuses before the store is asked, how each kind on the wire
becomes an operation, and how each refusal answers.
*/

// draftStub answers what it is told and records what it was asked.
type draftStub struct {
	err       error
	opened    drafts.Opened
	applied   []drafts.Application
	created   []*drafts.Identity
	callers   []drafts.Caller
	proposals []drafts.Proposal
	accepted  []drafts.Acceptance
	rejected  []string
	turns     []drafts.Turn
	// outcomesAsked is the turns each ProposalOutcomes call asked for.
	outcomesAsked [][]string
	// turnsRead is the ids each TurnsByID call read the content of.
	turnsRead [][]string
	begun     []drafts.Beginning
	beginErr  error
	stored    *drafts.Turn
}

// Seal is not exercised through the stub: sealing is tested against the real store.
func (d *draftStub) Seal(context.Context, drafts.Sealing) (drafts.Sealed, error) {
	return drafts.Sealed{}, drafts.ErrNotFound
}

// ThreadFor is not exercised through the stub: a rehearsal is tested against the real store.
func (d *draftStub) ThreadFor(context.Context, domain.AgentID, string, domain.UserID) (string, error) {
	return "", drafts.ErrNotFound
}

func (d *draftStub) Get(_ context.Context, _ domain.AgentID, c drafts.Caller) (drafts.Opened, error) {
	d.callers = append(d.callers, c)
	return d.opened, d.err
}

func (d *draftStub) Create(_ context.Context, _ domain.AgentID, id *drafts.Identity, c drafts.Caller) (drafts.Opened, error) {
	d.created = append(d.created, id)
	d.callers = append(d.callers, c)
	return d.opened, d.err
}

func (d *draftStub) Apply(_ context.Context, a drafts.Application) (drafts.Opened, error) {
	d.applied = append(d.applied, a)
	return d.opened, d.err
}

func (d *draftStub) Discard(_ context.Context, _ domain.AgentID, _ domain.IfMatch, c drafts.Caller) error {
	d.callers = append(d.callers, c)
	return d.err
}

func draftServer(stub *draftStub) *Server {
	return NewServer(ledger.NewMemory(), "test").WithDrafts(stub)
}

func someDraft() drafts.Opened {
	d := draft.New("triage", "Triagem", domain.Scope{Company: "acme", Area: "cx"})
	d.Blocks = blocks.Parse("Objetivo\nTriar.")
	return drafts.Opened{
		Agent: "triage", DraftID: "d-1", Draft: d, Revision: `"v1-draft"`,
		Missing: nil,
	}
}

func TestCreateAgentDraft_withoutIfNoneMatch_isRefusedBeforeTheStore(t *testing.T) {
	t.Parallel()
	stub := &draftStub{opened: someDraft()}
	resp, err := draftServer(stub).CreateAgentDraft(inArea("cx", domain.RoleAuthor), openapi.CreateAgentDraftRequestObject{
		AgentId: "triage", Body: &openapi.DraftIdentity{Name: "Triagem", Company: "acme", Area: "cx"},
	})
	if err != nil {
		t.Fatalf("CreateAgentDraft: %v", err)
	}
	if _, ok := resp.(openapi.CreateAgentDraft400ApplicationProblemPlusJSONResponse); !ok || len(stub.created) != 0 {
		t.Fatalf("response = %T (%d asked), want 400 before the store", resp, len(stub.created))
	}
}

// The caller the store judges with is the request's: agent:publish where the
// store asks.
func TestCreateAgentDraft_handsTheStoreTheCallersAuthority(t *testing.T) {
	t.Parallel()
	stub := &draftStub{opened: someDraft()}
	resp, err := draftServer(stub).CreateAgentDraft(inArea("cx", domain.RoleAuthor), openapi.CreateAgentDraftRequestObject{
		AgentId: "triage", Params: openapi.CreateAgentDraftParams{IfNoneMatch: "*"},
		Body: &openapi.DraftIdentity{Name: "Triagem", Company: "acme", Area: "cx"},
	})
	if err != nil {
		t.Fatalf("CreateAgentDraft: %v", err)
	}
	created, ok := resp.(openapi.CreateAgentDraft201JSONResponse)
	if !ok || created.Headers.ETag != `"v1-draft"` {
		t.Fatalf("response = %T %+v, want 201 with the ETag", resp, resp)
	}
	if len(stub.created) != 1 || stub.created[0] == nil || stub.created[0].Scope != (domain.Scope{Company: "acme", Area: "cx"}) {
		t.Fatalf("identity = %+v", stub.created)
	}
	allow := stub.callers[0].Allow
	if !allow(domain.Scope{Company: "acme", Area: "cx"}) || allow(domain.Scope{Company: "acme", Area: "ops"}) {
		t.Error("Allow does not answer agent:publish as the caller holds it")
	}
}

// Contract 3: the route refuses every block operation, without asking.
func TestApplyDraftOperation_aBlockOperation_isRefusedBeforeTheStore(t *testing.T) {
	t.Parallel()
	for _, kind := range []openapi.DraftOperationKind{
		openapi.DraftOperationKindInsertBlock, openapi.DraftOperationKindReplaceBlock, openapi.DraftOperationKindEditBlock, openapi.DraftOperationKindRemoveBlock, openapi.DraftOperationKindMoveBlock,
	} {
		stub := &draftStub{opened: someDraft()}
		resp, err := draftServer(stub).ApplyDraftOperation(inArea("cx", domain.RoleAuthor), openapi.ApplyDraftOperationRequestObject{
			AgentId: "triage", Params: openapi.ApplyDraftOperationParams{IfMatch: `"v1-draft"`},
			Body: &openapi.DraftOperation{Kind: kind},
		})
		if err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
		refused, ok := resp.(openapi.ApplyDraftOperation400ApplicationProblemPlusJSONResponse)
		if !ok || len(stub.applied) != 0 {
			t.Errorf("%s: response = %T (%d asked), want 400 before the store", kind, resp, len(stub.applied))
			continue
		}
		// Refused as contract 3 refuses it, not as an unknown kind.
		if refused.Detail == nil || !strings.Contains(*refused.Detail, "accepted proposal") {
			t.Errorf("%s: detail = %v, want contract 3's reason", kind, refused.Detail)
		}
	}
}

func TestApplyDraftOperation_withoutIfMatch_isRefusedBeforeTheStore(t *testing.T) {
	t.Parallel()
	stub := &draftStub{opened: someDraft()}
	resp, err := draftServer(stub).ApplyDraftOperation(inArea("cx", domain.RoleAuthor), openapi.ApplyDraftOperationRequestObject{
		AgentId: "triage", Body: &openapi.DraftOperation{Kind: openapi.DraftOperationKindAddTool, Tool: ptr("crm.lookup")},
	})
	if err != nil {
		t.Fatalf("ApplyDraftOperation: %v", err)
	}
	if _, ok := resp.(openapi.ApplyDraftOperation400ApplicationProblemPlusJSONResponse); !ok || len(stub.applied) != 0 {
		t.Fatalf("response = %T, want 400 before the store", resp)
	}
}

// Each kind on the wire becomes the operation of that kind, reading only its
// own fields.
func TestApplyDraftOperation_eachKind_becomesItsOperation(t *testing.T) {
	t.Parallel()
	cases := []struct {
		body openapi.DraftOperation
		want draft.Operation
	}{
		{openapi.DraftOperation{Kind: openapi.DraftOperationKindSetIdentity, Name: ptr("T"), Company: ptr("acme"), Area: ptr("ops")},
			draft.SetIdentity{Name: "T", Scope: domain.Scope{Company: "acme", Area: "ops"}}},
		{openapi.DraftOperation{Kind: openapi.DraftOperationKindSetModel, Provider: ptr("openai"), Model: ptr("m"), Effort: ptr("high")},
			draft.SetModel{Provider: "openai", Model: "m", Effort: "high"}},
		{openapi.DraftOperation{Kind: openapi.DraftOperationKindSetFallback}, draft.SetFallback{}},
		{openapi.DraftOperation{Kind: openapi.DraftOperationKindSetReplyLocale, ReplyLocale: ptr("en-US"), Tool: ptr("ignored")},
			draft.SetReplyLocale{Locale: "en-US"}},
		{openapi.DraftOperation{Kind: openapi.DraftOperationKindAddTool, Tool: ptr("crm.lookup")}, draft.AddTool{Tool: "crm.lookup"}},
		{openapi.DraftOperation{Kind: openapi.DraftOperationKindRemoveStep, StepName: ptr("Look")}, draft.RemoveStep{Name: "Look"}},
		{openapi.DraftOperation{Kind: openapi.DraftOperationKindSetStandingApprovals, StandingApprovals: ptr(openapi.StandingPolicy("forbidden"))},
			draft.SetStandingApprovals{Policy: domain.StandingForbidden}},
	}
	for _, tc := range cases {
		stub := &draftStub{opened: someDraft()}
		body := tc.body
		if _, err := draftServer(stub).ApplyDraftOperation(inArea("cx", domain.RoleAuthor), openapi.ApplyDraftOperationRequestObject{
			AgentId: "triage", Params: openapi.ApplyDraftOperationParams{IfMatch: `"v1-draft"`}, Body: &body,
		}); err != nil {
			t.Fatalf("%s: %v", tc.body.Kind, err)
		}
		if len(stub.applied) != 1 {
			t.Fatalf("%s: the store was asked %d times", tc.body.Kind, len(stub.applied))
		}
		got := stub.applied[0]
		if draft.Digest(got.Op) != draft.Digest(tc.want) || got.Origin != draft.FromControl {
			t.Errorf("%s became %#v (%v), want %#v by hand", tc.body.Kind, got.Op, got.Origin, tc.want)
		}
	}
}

// Every refusal the store gives answers as the contract says.
func TestApplyDraftOperation_refusals_answerAsTheContractSays(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		err  error
		want string
	}{
		"not found":            {drafts.ErrNotFound, "404"},
		"forbidden":            {&drafts.ForbiddenError{Scope: domain.Scope{Company: "acme", Area: "ops"}}, "403"},
		"stale":                {&domain.StaleError{Current: `"v1-now"`}, "412"},
		"refused":              {draft.ErrRefused, "400"},
		"invalid":              {draft.ErrInvalidOperation, "400"},
		"too large":            {draft.ErrTooLarge, "400"},
		"conversation only":    {draft.ErrConversationOnly, "400"},
		"needs a precondition": {drafts.ErrPreconditionRequired, "400"},
	}
	for name, tc := range cases {
		stub := &draftStub{err: tc.err}
		resp, err := draftServer(stub).ApplyDraftOperation(inArea("cx", domain.RoleAuthor), openapi.ApplyDraftOperationRequestObject{
			AgentId: "triage", Params: openapi.ApplyDraftOperationParams{IfMatch: `"v1-draft"`},
			Body: &openapi.DraftOperation{Kind: openapi.DraftOperationKindAddTool, Tool: ptr("crm.lookup")},
		})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		var got string
		switch r := resp.(type) {
		case openapi.ApplyDraftOperation404ApplicationProblemPlusJSONResponse:
			got = "404"
		case openapi.ApplyDraftOperation403ApplicationProblemPlusJSONResponse:
			got = "403"
		case openapi.ApplyDraftOperation412ApplicationProblemPlusJSONResponse:
			got = "412"
			if r.Headers.ETag == nil || *r.Headers.ETag != `"v1-now"` {
				t.Errorf("%s: ETag = %v, want the current one", name, r.Headers.ETag)
			}
		case openapi.ApplyDraftOperation400ApplicationProblemPlusJSONResponse:
			got = "400"
		default:
			got = "other"
		}
		if got != tc.want {
			t.Errorf("%s: answered %T, want %s", name, resp, tc.want)
		}
	}
}

func TestCreateAgentDraft_anIdentityForAPublishedAgent_isAConflict(t *testing.T) {
	t.Parallel()
	stub := &draftStub{err: drafts.ErrAgentExists}
	resp, err := draftServer(stub).CreateAgentDraft(inArea("cx", domain.RoleAuthor), openapi.CreateAgentDraftRequestObject{
		AgentId: "triage", Params: openapi.CreateAgentDraftParams{IfNoneMatch: "*"},
		Body: &openapi.DraftIdentity{Name: "Triagem", Company: "acme", Area: "cx"},
	})
	if err != nil {
		t.Fatalf("CreateAgentDraft: %v", err)
	}
	if _, ok := resp.(openapi.CreateAgentDraft409ApplicationProblemPlusJSONResponse); !ok {
		t.Fatalf("response = %T, want 409", resp)
	}
}

// The read shows the blocks a proposal will name, never their raw bytes, and
// the definition with its instructions rendered.
func TestGetAgentDraft_showsTheDefinitionBlocksAndWhatIsMissing(t *testing.T) {
	t.Parallel()
	opened := someDraft()
	opened.Missing = nil
	stub := &draftStub{opened: opened}
	resp, err := draftServer(stub).GetAgentDraft(inArea("cx", domain.RoleAuthor), openapi.GetAgentDraftRequestObject{AgentId: "triage"})
	if err != nil {
		t.Fatalf("GetAgentDraft: %v", err)
	}
	got, ok := resp.(openapi.GetAgentDraft200JSONResponse)
	if !ok || got.Headers.ETag != `"v1-draft"` {
		t.Fatalf("response = %T, want 200 with the ETag", resp)
	}
	if got.Body.Definition.Instructions != "Objetivo\nTriar." || got.Body.Definition.Area != "cx" {
		t.Errorf("definition = %+v", got.Body.Definition)
	}
	if len(got.Body.Blocks) != 1 || got.Body.Blocks[0].Id != "b1" || got.Body.Blocks[0].Kind != openapi.DraftBlockKindObjective ||
		got.Body.Blocks[0].Text != "Triar." {
		t.Errorf("blocks = %+v", got.Body.Blocks)
	}
	if got.Body.Missing == nil {
		t.Error("missing is null; the contract says an array")
	}
	stub.err = drafts.ErrNotFound
	if resp, _ := draftServer(stub).GetAgentDraft(inArea("cx", domain.RoleAuthor), openapi.GetAgentDraftRequestObject{AgentId: "triage"}); resp == nil {
		t.Fatal("no answer")
	} else if _, ok := resp.(openapi.GetAgentDraft404ApplicationProblemPlusJSONResponse); !ok {
		t.Errorf("not found = %T, want 404", resp)
	}
}

// The route hands the store the installation's own checks: an operation the
// publication would refuse for a tool this installation does not offer is
// refused by the validator the route passes.
func TestApplyDraftOperation_validatesWithTheInstallationsChecks(t *testing.T) {
	t.Parallel()
	stub := &draftStub{opened: someDraft()}
	server := draftServer(stub).WithAdministration(nil, publishFlowTools(), nil)
	if _, err := server.ApplyDraftOperation(inArea("cx", domain.RoleAuthor), openapi.ApplyDraftOperationRequestObject{
		AgentId: "triage", Params: openapi.ApplyDraftOperationParams{IfMatch: `"v1-draft"`},
		Body: &openapi.DraftOperation{Kind: openapi.DraftOperationKindAddTool, Tool: ptr("crm.lokup")},
	}); err != nil {
		t.Fatalf("ApplyDraftOperation: %v", err)
	}
	a := stub.applied[0]
	if _, err := a.Validator.Validate(context.Background(), someDraft().Draft, a.Op, a.Origin, a.Now); !errors.Is(err, draft.ErrRefused) {
		t.Fatalf("the route's validator = %v, want the tool refused", err)
	}
}
