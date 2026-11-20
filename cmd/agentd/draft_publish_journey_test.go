package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/authoring/blocks"
	"github.com/fuseone/agents/internal/authoring/draft"
	"github.com/fuseone/agents/internal/authoring/drafts"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/httpapi"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/publishable"
	"github.com/fuseone/agents/internal/spec"
	"github.com/fuseone/agents/internal/testdb"
)

/*
Publishing from the workbench, through the router, the publication and
Postgres (NT-018, contracts 4, 5 and 10). A draft with no "never" block is
refused; one read at an older ETag is a 412; one whose agent was published
elsewhere since is a 412 that says so; one published becomes the agent's
version, born paused, and the draft stays, based on it.
*/

type publishWorld struct {
	draftJourney
	store *drafts.Store
	agent string
}

func newPublishWorld(t *testing.T) publishWorld {
	t.Helper()
	pool := testdb.Pool(t)
	store := drafts.NewStore(pool)
	api := httpapi.NewServer(ledger.NewMemory(), "test").
		WithAgents(spec.NewRegistry(pool)).
		WithPublisher(spec.NewPublisher(pool, engine.SystemClock{})).
		WithRetirements(spec.NewState(pool)).
		WithDrafts(store).
		WithCandidates(spec.NewCandidateReader(pool, ledger.NewContent(pool)))
	return publishWorld{
		draftJourney: draftJourney{etagJourney{handler: apiHandlerFor(api)}},
		store:        store, agent: fmt.Sprintf("nt018-publish-%d", time.Now().UnixNano()),
	}
}

// ready is a draft every publication check passes but the "never" block.
func (w publishWorld) ready(t *testing.T) string {
	t.Helper()
	created := w.create(t, areaAuthor("factory"), w.agent, "factory")
	wantStatus(t, "create", created, http.StatusCreated)
	tag := created.Header().Get("ETag")
	for _, op := range []string{
		`{"kind":"set_model","provider":"openai","model":"gpt-test"}`,
		`{"kind":"add_tool","tool":"crm.lookup"}`,
		`{"kind":"set_budget","budget":{"micros":500000,"steps":60}}`,
	} {
		rec := w.apply(t, areaAuthor("factory"), w.agent, tag, op)
		wantStatus(t, op, rec, http.StatusOK)
		tag = rec.Header().Get("ETag")
	}
	return w.accept(t, tag, draft.InsertBlock{At: 0, Type: blocks.Objective, Text: "Triar chamados.", Locale: "pt-BR"})
}

// accept proposes an operation in Ana's thread and accepts it, answering the
// draft's next ETag.
func (w publishWorld) accept(t *testing.T, tag string, op draft.Operation) string {
	t.Helper()
	p, err := w.store.Propose(t.Context(), drafts.Proposing{Agent: domain.AgentID(w.agent), Op: op, Read: domain.Revision(tag),
		Caller:    drafts.Caller{By: "usr_ana", Allow: func(domain.Scope) bool { return true }},
		Validator: draft.Validator{Checker: publishable.Checker{}}, Now: time.Now()})
	if err != nil {
		t.Fatalf("Propose: %v", err)
	}
	rec := w.do(t, areaAuthor("factory"), http.MethodPost, "/api/v1/agents/"+w.agent+"/draft/proposals/"+p.ID+"/accept", nil, nil)
	wantStatus(t, "accept", rec, http.StatusOK)
	return rec.Header().Get("ETag")
}

func (w publishWorld) publish(t *testing.T, caller domain.Principal, tag string) *httptest.ResponseRecorder {
	t.Helper()
	return w.do(t, caller, http.MethodPost, "/api/v1/agents/"+w.agent+"/draft/publish", nil, map[string]string{"If-Match": tag})
}

func problemType(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var p struct{ Type string }
	_ = json.Unmarshal(rec.Body.Bytes(), &p)
	return p.Type
}

func TestDraftPublish_aNewAgent_needsItsNeverBlockAndItsETag_thenIsBornPaused(t *testing.T) {
	w := newPublishWorld(t)
	tag := w.ready(t)
	refused := w.publish(t, areaAuthor("factory"), tag)
	if wantStatus(t, "without never", refused, http.StatusBadRequest); problemType(t, refused) != "fuseone:publish-needs-never" {
		t.Errorf("refusal type %q", problemType(t, refused))
	}
	older := tag
	tag = w.accept(t, tag, draft.InsertBlock{At: 1, Type: blocks.Never, Text: "Prometer prazo.", Locale: "pt-BR"})
	wantStatus(t, "an older ETag", w.publish(t, areaAuthor("factory"), older), http.StatusPreconditionFailed)
	wantStatus(t, "another area's author", w.publish(t, areaAuthor("ops"), tag), http.StatusNotFound)

	rec := w.publish(t, areaAuthor("factory"), tag)
	wantStatus(t, "publish", rec, http.StatusOK)
	var resp openapi.PublishAgentDraft200JSONResponse
	got := &resp.Body
	decode(t, rec, got)
	if !got.Created || !got.Paused || got.Draft.BaseVersionId == nil || *got.Draft.BaseVersionId != got.VersionId {
		t.Errorf("published = %+v, want a new version, paused, the draft based on it", got)
	}
}

func TestDraftPublish_anAgentPublishedElsewhereSince_isRefusedAsMoved(t *testing.T) {
	w := newPublishWorld(t)
	tag := w.accept(t, w.ready(t), draft.InsertBlock{At: 1, Type: blocks.Never, Text: "Prometer prazo.", Locale: "pt-BR"})
	wantStatus(t, "first", w.publish(t, areaAuthor("factory"), tag), http.StatusOK)
	agentTag := w.tag(t, areaAuthor("factory"), w.agent)
	wantStatus(t, "elsewhere", w.put(t, areaAuthor("factory"), w.agent, agentTag, "", "Nunca\nOutra coisa."), http.StatusOK)

	moved := w.publish(t, areaAuthor("factory"), tag)
	if wantStatus(t, "after elsewhere", moved, http.StatusPreconditionFailed); problemType(t, moved) != "fuseone:agent-moved" {
		t.Errorf("refusal type %q, want agent-moved", problemType(t, moved))
	}
}

/*
A publication committed is answered as such, whatever happens to the draft after the
commit: the answer is the version and the draft as published, never read again. A
discard sent while the publication holds the draft waits for its commit (#416 moved
everything a publication does inside it), lands after it, and the answer is still the
version and the draft as published.
*/
func TestDraftPublish_aDraftDiscardedAfterTheCommit_isStillAnsweredAsPublished(t *testing.T) {
	var stall stallingSchedules
	w, _ := reconcilingWorld(t, stalling(&stall))
	tag := w.accept(t, w.ready(t), draft.InsertBlock{At: 1, Type: blocks.Never, Text: "Prometer prazo.", Locale: "pt-BR"})

	answered := make(chan *httptest.ResponseRecorder, 1)
	go func() { answered <- w.publish(t, areaAuthor("factory"), tag) }()
	<-stall.entered
	discarded := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		discarded <- w.do(t, areaAuthor("factory"), http.MethodDelete, "/api/v1/agents/"+w.agent+"/draft", nil, map[string]string{"If-Match": tag})
	}()
	close(stall.release)

	rec := <-answered
	wantStatus(t, "publish", rec, http.StatusOK)
	wantStatus(t, "discard", <-discarded, http.StatusNoContent)
	var resp openapi.PublishAgentDraft200JSONResponse
	decode(t, rec, &resp.Body)
	if resp.Body.Draft.BaseVersionId == nil || *resp.Body.Draft.BaseVersionId != resp.Body.VersionId || rec.Header().Get("ETag") != tag {
		t.Errorf("answer = %+v (ETag %q), want the draft as published, based on the version", resp.Body, rec.Header().Get("ETag"))
	}
}
