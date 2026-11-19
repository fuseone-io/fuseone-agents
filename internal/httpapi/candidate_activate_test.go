package httpapi

import (
	gocontext "context"
	"errors"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/spec"
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
}

func newFakeActivations(corpus *fakeCorpus) *fakeActivations {
	return &fakeActivations{
		scopes: []domain.Scope{cxScope}, corpus: corpus,
		activated: spec.Activated{
			Spec: spec.Spec{ID: "triage", Version: activatedVersion, Company: "acme", Area: "cx",
				Instructions: "Classify.\n\nNever\nRefund."},
			SealID: "0b7c3e52-8a43-4b55-9a3f-0d6a3c5f9e11", Created: true,
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
	return spec.Spec{}, false, nil
}

func (f *fakeActivations) Activate(_ gocontext.Context, a spec.Activation) (spec.Activated, error) {
	f.calls, f.fenced = append(f.calls, a), f.corpus.inside
	if f.err != nil {
		return spec.Activated{}, f.err
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

func TestActivate_committed_answersTheNewTagFromInsideTheFence_andRecordsItPaused(t *testing.T) {
	t.Parallel()
	corpus, pub := &fakeCorpus{}, newPublisher()
	f := newFakeActivations(corpus)

	resp := activate(t, activationServer(f, pub), inArea("cx", domain.RoleAuthor), liveTag(), nil)
	ok, isOK := resp.(openapi.ActivateAgentVersion200JSONResponse)
	if !isOK || ok.Headers.ETag != string(f.activated.Revision) || !ok.Body.Created ||
		ok.Body.SealId == nil || *ok.Body.SealId != f.activated.SealID || !ok.Body.Paused {
		t.Fatalf("answer = %T %+v, want 200 with the new tag, the seal and paused", resp, resp)
	}
	a := f.calls[0]
	if !f.fenced || a.Scope != cxScope || a.By != "usr_ana" || a.Guard.Allow == nil {
		t.Errorf("activation = %+v (fenced %v), want it inside the fence, in cx, by the caller, guarded", a, f.fenced)
	}
	if len(pub.ensured) != 1 || pub.ensured[0] != "triage" {
		t.Errorf("ensured paused = %v, want the agent, as after a publication", pub.ensured)
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

func TestActivate_storeRefusals_becomeThePublicationsAnswers(t *testing.T) {
	t.Parallel()
	conflicts := map[error]Code{
		spec.ErrCandidateScopeMoved:   CodeCandidateScopeMoved,
		spec.ErrStageRefused:          CodeStageRefused,
		domain.ErrCandidateUnreadable: CodeCandidateUnreadable,
		spec.ErrVersionCollision:      CodeVersionCollision,
		spec.ErrDigestUnknown:         CodeVersionDigestUnknown,
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
