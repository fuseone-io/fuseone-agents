package httpapi

import (
	gocontext "context"
	"errors"
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/model"
	"github.com/fuseone/agents/internal/spec"
	"github.com/fuseone/agents/internal/trigger"
)

/*
The activation route (NT-018 phase 2, release 2c), around the store's own steps.

Before any lock it reads only headers, by syntax (428 for neither, 400 for both or one
that does not parse), then the version's scope, which the caller must be able to publish
in (404 otherwise). Everything after that runs inside the agent's content fence. The
store's refusals become the answers a publication gives for the same causes, the never
block is the store's to judge through the route's acceptance, and a committed activation
answers the agent's new tag.
*/

const activatedVersion = domain.VersionID("vcand0000001")

// fakeActivations answers one version in cx, and records what it was asked to activate.
type fakeActivations struct {
	scopes    []domain.Scope
	activated spec.Activated
	err       error
	corpus    *fakeCorpus
	calls     []spec.Activation
	fenced    bool
	// unreadable is a candidate with no readable seal before the locks.
	unreadable bool
	// answerRefused has the store answer what the checks carried, as it does once
	// nothing it judges first refuses.
	answerRefused bool
}

func newFakeActivations(corpus *fakeCorpus) *fakeActivations {
	return &fakeActivations{
		scopes: []domain.Scope{cxScope}, corpus: corpus,
		activated: spec.Activated{
			Spec: spec.Spec{ID: "triage", Version: activatedVersion, Company: "acme", Area: "cx",
				Instructions: "Classify.\n\nNever\nRefund."},
			SealID: "0b7c3e52-8a43-4b55-9a3f-0d6a3c5f9e11", Created: true, Paused: true,
			Revision: domain.AgentRevision(activatedVersion, false),
		},
	}
}

func (f *fakeActivations) Scopes(gocontext.Context, domain.AgentID, domain.VersionID) ([]domain.Scope, error) {
	return f.scopes, nil
}

func (f *fakeActivations) Definition(
	gocontext.Context, domain.AgentID, domain.VersionID, domain.Scope,
) (spec.Spec, bool, error) {
	return f.activated.Spec, !f.unreadable, nil
}

func (f *fakeActivations) Activate(_ gocontext.Context, a spec.Activation) (spec.Activated, error) {
	f.calls, f.fenced = append(f.calls, a), f.corpus.inside
	if f.err != nil {
		return spec.Activated{}, f.err
	}
	if f.answerRefused && a.Refused != nil {
		return spec.Activated{}, a.Refused
	}
	if err := a.Accept(f.activated.Spec); err != nil {
		return spec.Activated{}, err
	}
	return f.activated, nil
}

func activationServer(f *fakeActivations, pub *publisher) *Server {
	return NewServer(ledger.NewMemory(), "test").WithPublisher(pub).
		WithRegressions(f.corpus).WithActivations(f)
}

func activate(t *testing.T, s *Server, ctx gocontext.Context, ifMatch, ifNoneMatch *string) openapi.ActivateAgentVersionResponseObject {
	t.Helper()
	resp, err := s.ActivateAgentVersion(ctx, openapi.ActivateAgentVersionRequestObject{
		AgentId: "triage", Version: string(activatedVersion),
		Params: openapi.ActivateAgentVersionParams{IfMatch: ifMatch, IfNoneMatch: ifNoneMatch},
	})
	if err != nil {
		t.Fatalf("ActivateAgentVersion: %v", err)
	}
	return resp
}

func liveTag() *string { return ptr(string(domain.AgentRevision("vlive0000001", false))) }

func TestActivate_withNeitherHeader_isPreconditionRequired_beforeAnythingIsRead(t *testing.T) {
	t.Parallel()
	f := newFakeActivations(&fakeCorpus{})
	f.scopes = nil

	resp := activate(t, activationServer(f, newPublisher()), inArea("cx", domain.RoleAuthor), nil, nil)
	required, ok := resp.(openapi.ActivateAgentVersion428ApplicationProblemPlusJSONResponse)
	if !ok || required.Type == nil || *required.Type != string(CodePreconditionRequired) {
		t.Fatalf("answer = %T %+v, want 428 precondition-required", resp, resp)
	}
}

func TestActivate_withBothHeadersOrOneThatDoesNotParse_isABadRequest(t *testing.T) {
	t.Parallel()
	for name, headers := range map[string][2]*string{
		"both":         {liveTag(), ptr("*")},
		"bad if-match": {ptr("not a tag"), nil},
		"bad if-none":  {nil, ptr(`"x"`)},
	} {
		f := newFakeActivations(&fakeCorpus{})
		resp := activate(t, activationServer(f, newPublisher()), inArea("cx", domain.RoleAuthor), headers[0], headers[1])
		if _, ok := resp.(openapi.ActivateAgentVersion400ApplicationProblemPlusJSONResponse); !ok || len(f.calls) != 0 {
			t.Errorf("%s: answer = %T, %d activations; want 400 and none", name, resp, len(f.calls))
		}
	}
}

func TestActivate_aVersionTheCallerMayNotPublishIn_orNone_isNotFound(t *testing.T) {
	t.Parallel()
	for name, scopes := range map[string][]domain.Scope{
		"none":       nil,
		"elsewhere":  {{Company: "acme", Area: "finance"}},
		"one of two": {cxScope, {Company: "acme", Area: "finance"}},
	} {
		f := newFakeActivations(&fakeCorpus{})
		f.scopes = scopes
		resp := activate(t, activationServer(f, newPublisher()), inArea("cx", domain.RoleAuthor), liveTag(), nil)
		if _, ok := resp.(openapi.ActivateAgentVersion404ApplicationProblemPlusJSONResponse); !ok || len(f.calls) != 0 {
			t.Errorf("%s: answer = %T, %d activations; want 404 and none", name, resp, len(f.calls))
		}
	}
}

func TestActivate_committed_answersTheNewTagFromInsideTheFence_withNothingLeftForAfter(t *testing.T) {
	t.Parallel()
	corpus, pub := &fakeCorpus{}, newPublisher()
	f := newFakeActivations(corpus)

	resp := activate(t, activationServer(f, pub), inArea("cx", domain.RoleAuthor), liveTag(), nil)
	ok, isOK := resp.(openapi.ActivateAgentVersion200JSONResponse)
	if !isOK || ok.Headers.ETag != string(f.activated.Revision) || !ok.Body.Created ||
		ok.Body.SealId == nil || *ok.Body.SealId != f.activated.SealID || !ok.Body.Paused {
		t.Fatalf("answer = %T %+v, want 200 with the new tag, the seal and the pause the store read", resp, resp)
	}
	a := f.calls[0]
	if !f.fenced || a.Scope != cxScope || a.By != "usr_ana" || a.Guard.Allow == nil {
		t.Errorf("activation = %+v (fenced %v), want it inside the fence, in cx, by the caller, guarded", a, f.fenced)
	}
	// Schedules, webhooks and the pause move in the store's transaction: anything after
	// the commit could land after a later activation's.
	if len(pub.ensured) != 0 {
		t.Errorf("ensured paused after the commit = %v, want nothing done after it", pub.ensured)
	}
}

/*
A candidate unreadable before the locks is not answered there: the store judges the
agent's scope, the tag, the scope move and the stage first, and refuses unchecked bytes
only after them. The route says it checked nothing, and answers what the store found.
*/
func TestActivate_aCandidateUnreadableBeforeTheLocks_isTheStoresToAnswer_unchecked(t *testing.T) {
	t.Parallel()
	current := domain.AgentRevision("vother000001", false)
	for name, c := range map[string]struct {
		err  error
		want func(openapi.ActivateAgentVersionResponseObject) bool
	}{
		"outside the scope": {spec.ErrAgentNotFound, func(r openapi.ActivateAgentVersionResponseObject) bool {
			_, ok := r.(openapi.ActivateAgentVersion404ApplicationProblemPlusJSONResponse)
			return ok
		}},
		"a stale tag": {&domain.StaleError{Current: current}, func(r openapi.ActivateAgentVersionResponseObject) bool {
			_, ok := r.(openapi.ActivateAgentVersion412ApplicationProblemPlusJSONResponse)
			return ok
		}},
		"unchecked": {spec.ErrNotChecked, func(r openapi.ActivateAgentVersionResponseObject) bool {
			c, ok := r.(openapi.ActivateAgentVersion409ApplicationProblemPlusJSONResponse)
			return ok && c.Type != nil && *c.Type == string(CodeCandidateUnreadable)
		}},
	} {
		f := newFakeActivations(&fakeCorpus{})
		f.unreadable, f.err = true, c.err
		resp := activate(t, activationServer(f, newPublisher()), inArea("cx", domain.RoleAuthor), liveTag(), nil)
		if len(f.calls) != 1 || f.calls[0].Checked || !c.want(resp) {
			t.Errorf("%s: answer = %T, %d activations (checked %v); want the store's answer, unchecked",
				name, resp, len(f.calls), len(f.calls) == 1 && f.calls[0].Checked)
		}
	}
}

func TestActivate_aDefinitionReadBeforeTheLocks_isActivatedChecked(t *testing.T) {
	t.Parallel()
	f := newFakeActivations(&fakeCorpus{})

	activate(t, activationServer(f, newPublisher()), inArea("cx", domain.RoleAuthor), liveTag(), nil)
	if len(f.calls) != 1 || !f.calls[0].Checked {
		t.Fatalf("activations = %+v, want one, checked", f.calls)
	}
}

func TestActivate_aDefinitionWithNoNeverBlock_isRefused(t *testing.T) {
	t.Parallel()
	f := newFakeActivations(&fakeCorpus{})
	f.activated.Spec.Instructions = "Classify."

	resp := activate(t, activationServer(f, newPublisher()), inArea("cx", domain.RoleAuthor), liveTag(), nil)
	bad, ok := resp.(openapi.ActivateAgentVersion400ApplicationProblemPlusJSONResponse)
	if !ok || bad.Type == nil || *bad.Type != string(CodePublishNeedsNever) {
		t.Fatalf("answer = %T %+v, want 400 publish-needs-never", resp, resp)
	}
}

// A candidate is published from the workbench, so the workbench's checklist
// holds there too: a tool no step reaches would never run.
func TestActivate_aToolNoStepReaches_isRefusedNamingIt(t *testing.T) {
	t.Parallel()
	f := newFakeActivations(&fakeCorpus{})
	f.activated.Spec.Tools = []domain.ToolID{"crm.lookup", "rootly.create_alert"}
	f.activated.Spec.Steps = []spec.Step{{Name: "Look up", Reaches: []domain.ToolID{"crm.lookup"}}}

	resp := activate(t, activationServer(f, newPublisher()), inArea("cx", domain.RoleAuthor), liveTag(), nil)
	bad, ok := resp.(openapi.ActivateAgentVersion400ApplicationProblemPlusJSONResponse)
	if !ok || bad.Type == nil || *bad.Type != string(CodePublishUnplacedTools) ||
		bad.Detail == nil || !strings.Contains(*bad.Detail, "rootly.create_alert") {
		t.Fatalf("answer = %T %+v, want 400 publish-unplaced-tools naming the tool", resp, resp)
	}
}

func TestActivate_storeRefusals_becomeThePublicationsAnswers(t *testing.T) {
	t.Parallel()
	conflicts := map[error]Code{
		spec.ErrCandidateScopeMoved:   CodeCandidateScopeMoved,
		spec.ErrStageRefused:          CodeStageRefused,
		domain.ErrCandidateUnreadable: CodeCandidateUnreadable,
		spec.ErrVersionCollision:      CodeVersionCollision,
		spec.ErrDigestUnknown:         CodeVersionDigestUnknown,
		trigger.ErrPathTaken:          CodeWebhookPathTaken,
	}
	for err, code := range conflicts {
		f := newFakeActivations(&fakeCorpus{})
		f.err = errors.Join(errors.New("wrapped"), err)
		resp := activate(t, activationServer(f, newPublisher()), inArea("cx", domain.RoleAuthor), liveTag(), nil)
		c, ok := resp.(openapi.ActivateAgentVersion409ApplicationProblemPlusJSONResponse)
		if !ok || c.Type == nil || *c.Type != string(code) {
			t.Errorf("%v: answer = %T %+v, want 409 %s", err, resp, resp, code)
		}
	}
	for _, err := range []error{spec.ErrAgentNotFound, spec.ErrCandidateNotFound} {
		f := newFakeActivations(&fakeCorpus{})
		f.err = err
		resp := activate(t, activationServer(f, newPublisher()), inArea("cx", domain.RoleAuthor), liveTag(), nil)
		if _, ok := resp.(openapi.ActivateAgentVersion404ApplicationProblemPlusJSONResponse); !ok {
			t.Errorf("%v: answer = %T, want 404", err, resp)
		}
	}
}

func TestActivate_aStaleTag_answersTheCurrentOne(t *testing.T) {
	t.Parallel()
	f := newFakeActivations(&fakeCorpus{})
	current := domain.AgentRevision("vother000001", false)
	f.err = &domain.StaleError{Current: current}

	resp := activate(t, activationServer(f, newPublisher()), inArea("cx", domain.RoleAuthor), liveTag(), nil)
	stale, ok := resp.(openapi.ActivateAgentVersion412ApplicationProblemPlusJSONResponse)
	if !ok || stale.Headers.ETag == nil || *stale.Headers.ETag != string(current) {
		t.Fatalf("answer = %T %+v, want 412 with the current tag", resp, resp)
	}
}

// failingTools is a catalogue that cannot be read.
type failingTools struct{}

func (failingTools) Tools(gocontext.Context) ([]domain.ToolEntry, error) {
	return nil, errors.New("the catalogue is unreachable")
}

/*
What the publication's checks find before the locks — a refusal, or no answer at all — is
carried to the store and answered only after the agent's scope, the tag, the scope move
and the stage. An invalid definition never answers 400 to a caller the store would answer
404, nor hides a stale tag.
*/
func TestActivate_whatTheChecksFound_isAnsweredAfterTheStoresFirstDecisions(t *testing.T) {
	t.Parallel()
	refusing := func(f *fakeActivations) *Server {
		return activationServer(f, newPublisher()).WithProviderNames(model.NewRegistry(nil))
	}
	failing := func(f *fakeActivations) *Server {
		return activationServer(f, newPublisher()).WithAdministration(nil, failingTools{}, nil)
	}
	for name, c := range map[string]struct {
		server func(*fakeActivations) *Server
		store  error
		want   func(openapi.ActivateAgentVersionResponseObject) bool
	}{
		"refused, outside the scope": {refusing, spec.ErrAgentNotFound, isAbsent},
		"refused, a stale tag":       {refusing, &domain.StaleError{Current: domain.AgentRevision("vother000001", false)}, isStale},
		"failed, outside the scope":  {failing, spec.ErrAgentNotFound, isAbsent},
	} {
		f := newFakeActivations(&fakeCorpus{})
		f.activated.Spec.Provider, f.err = "openai", c.store
		resp := activate(t, c.server(f), inArea("cx", domain.RoleAuthor), liveTag(), nil)
		if len(f.calls) != 1 || f.calls[0].Refused == nil || !c.want(resp) {
			t.Errorf("%s: answer = %T, %d activations; want the store's answer, carrying what the checks found", name, resp, len(f.calls))
		}
	}
}

func TestActivate_aCheckRefusalTheStoreAnswers_isABadRequestWithItsDetail(t *testing.T) {
	t.Parallel()
	f := newFakeActivations(&fakeCorpus{})
	f.activated.Spec.Provider = "openai"
	server := activationServer(f, newPublisher()).WithProviderNames(model.NewRegistry(nil))
	f.answerRefused = true

	resp := activate(t, server, inArea("cx", domain.RoleAuthor), liveTag(), nil)
	bad, ok := resp.(openapi.ActivateAgentVersion400ApplicationProblemPlusJSONResponse)
	if !ok || bad.Type == nil || *bad.Type != string(CodeInvalidInput) || bad.Detail == nil || !strings.Contains(*bad.Detail, "openai") {
		t.Fatalf("answer = %T %+v, want 400 invalid-input naming the provider", resp, resp)
	}
}

func isAbsent(r openapi.ActivateAgentVersionResponseObject) bool {
	_, ok := r.(openapi.ActivateAgentVersion404ApplicationProblemPlusJSONResponse)
	return ok
}

func isStale(r openapi.ActivateAgentVersionResponseObject) bool {
	_, ok := r.(openapi.ActivateAgentVersion412ApplicationProblemPlusJSONResponse)
	return ok
}
