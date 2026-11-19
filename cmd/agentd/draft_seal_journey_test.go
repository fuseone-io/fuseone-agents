package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fuseone/agents/internal/authoring/blocks"
	"github.com/fuseone/agents/internal/authoring/draft"
	"github.com/fuseone/agents/internal/authoring/drafts"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/spec"
	"github.com/fuseone/agents/internal/testdb"
)

/*
Sealing the draft, through the router, the store and Postgres (NT-018 phase 2, release
2b). A draft every publication check passes is sealed even without its "never" block;
one read at an older ETag is a 412; another area's author gets 404; a draft that is not a
definition yet is a 400; a draft whose bytes are already published seals nothing and says
which version it is; and the seal it writes is a candidate the listing shows.
*/

func (w publishWorld) seal(t *testing.T, caller domain.Principal, tag string) *httptest.ResponseRecorder {
	t.Helper()
	return w.do(t, caller, http.MethodPost, "/api/v1/agents/"+w.agent+"/draft/seals", nil, map[string]string{"If-Match": tag})
}

func TestDraftSeal_aCheckedDraft_isSealedWithoutItsNeverBlock_andListedAsACandidate(t *testing.T) {
	w := newPublishWorld(t)
	tag := w.ready(t)
	older := tag
	tag = w.accept(t, tag, draft.InsertBlock{At: 1, Type: blocks.HowToAct, Text: "Ler o chamado inteiro.", Locale: "pt-BR"})

	wantStatus(t, "an older ETag", w.seal(t, areaAuthor("factory"), older), http.StatusPreconditionFailed)
	wantStatus(t, "another area's author", w.seal(t, areaAuthor("ops"), tag), http.StatusNotFound)

	rec := w.seal(t, areaAuthor("factory"), tag)
	wantStatus(t, "seal", rec, http.StatusCreated)
	var got openapi.DraftSeal
	decode(t, rec, &got)
	if got.Published || got.SealId == nil || *got.SealId == "" || got.Version == "" || got.Area != "factory" {
		t.Fatalf("sealed = %+v, want a new seal in factory", got)
	}

	listed := w.do(t, areaAuthor("factory"), http.MethodGet, "/api/v1/agents/"+w.agent+"/candidates", nil, nil)
	wantStatus(t, "list candidates", listed, http.StatusOK)
	var page struct {
		Items []openapi.CandidateSeal `json:"items"`
	}
	decode(t, listed, &page)
	if len(page.Items) != 1 || page.Items[0].SealId != *got.SealId || page.Items[0].Version != got.Version {
		t.Errorf("listed %+v, want the seal just written", page.Items)
	}
}

func TestDraftSeal_aDraftThatIsNotADefinitionYet_isABadRequest(t *testing.T) {
	w := newPublishWorld(t)
	created := w.create(t, areaAuthor("factory"), w.agent, "factory")
	wantStatus(t, "create", created, http.StatusCreated)

	wantStatus(t, "seal an empty draft", w.seal(t, areaAuthor("factory"), created.Header().Get("ETag")), http.StatusBadRequest)
}

func TestDraftSeal_aDraftAlreadyPublished_sealsNothingAndNamesTheVersion(t *testing.T) {
	w := newPublishWorld(t)
	tag := w.ready(t)
	tag = w.accept(t, tag, draft.InsertBlock{At: 1, Type: blocks.Never, Text: "Prometer prazo.", Locale: "pt-BR"})
	published := w.publish(t, areaAuthor("factory"), tag)
	wantStatus(t, "publish", published, http.StatusOK)
	var pub openapi.PublishAgentDraft200JSONResponse
	decode(t, published, &pub.Body)

	rec := w.seal(t, areaAuthor("factory"), published.Header().Get("ETag"))
	wantStatus(t, "seal the published draft", rec, http.StatusOK)
	var got openapi.DraftSeal
	decode(t, rec, &got)
	if !got.Published || got.SealId != nil || got.Version != pub.Body.VersionId {
		t.Errorf("sealed = %+v, want nothing sealed and the published version %s", got, pub.Body.VersionId)
	}
}

func TestDraftSeal_aVersionIdNamingOtherBytes_isAConflictWithItsCode(t *testing.T) {
	w := newPublishWorld(t)
	tag := w.ready(t)
	opened, err := w.store.Get(t.Context(), domain.AgentID(w.agent),
		drafts.Caller{By: "usr_ana", Allow: func(domain.Scope) bool { return true }})
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	rendered, err := spec.Render(opened.Draft.Definition())
	if err != nil {
		t.Fatal(err)
	}
	version, err := spec.Parse("x", rendered)
	if err != nil {
		t.Fatal(err)
	}
	pool := testdb.Pool(t)
	if _, err := pool.Exec(t.Context(), `insert into agent_version_digests (agent_id, version_id, digest)
		values ($1, $2, repeat('c', 64))`, w.agent, string(version.Version)); err != nil {
		t.Fatalf("claim other bytes: %v", err)
	}

	rec := w.seal(t, areaAuthor("factory"), tag)
	if wantStatus(t, "seal over a collision", rec, http.StatusConflict); problemType(t, rec) != "fuseone:version-collision" {
		t.Errorf("conflict type %q, want fuseone:version-collision", problemType(t, rec))
	}
}
