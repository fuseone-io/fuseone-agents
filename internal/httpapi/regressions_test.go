package httpapi

import (
	gocontext "context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
)

// A correction is only worth taking if a future version is checked against it
// without a person reading the run again. These are about the correction being
// kept in a form that survives the run it came from.

type fakeCorpus struct {
	recorded []domain.RegressionCase
	listed   []domain.RegressionCase
	err      error
	held     int
	inside   bool
}

func (f *fakeCorpus) Record(_ gocontext.Context, c domain.RegressionCase) error {
	if f.err != nil {
		return f.err
	}
	f.recorded = append(f.recorded, c)
	return nil
}

func (f *fakeCorpus) List(gocontext.Context, domain.AgentID) ([]domain.RegressionCase, error) {
	return f.listed, nil
}

func (f *fakeCorpus) Delete(gocontext.Context, domain.AgentID, string) error { return nil }

func (f *fakeCorpus) Hold(
	ctx gocontext.Context, _ domain.AgentID, inside func(gocontext.Context) error,
) error {
	f.held, f.inside = f.held+1, true
	defer func() { f.inside = false }()
	return inside(ctx)
}

// fencedContent records whether the corpus is being read and written from
// inside the fence an erasure takes.
type fencedContent struct {
	*engine.MemoryContent
	corpus  *fakeCorpus
	outside []string
}

func (c *fencedContent) RestoreFor(
	ctx gocontext.Context, kind, owner string, seq int64, data []byte,
) (string, error) {
	if !c.corpus.inside {
		c.outside = append(c.outside, "RestoreFor")
	}
	return c.MemoryContent.RestoreFor(ctx, kind, owner, seq, data)
}

func (c *fencedContent) Get(ctx gocontext.Context, ref string) ([]byte, error) {
	if !c.corpus.inside {
		c.outside = append(c.outside, "Get")
	}
	return c.MemoryContent.Get(ctx, ref)
}

// correctable returns a server holding one run with an occurrence behind it.
func correctable(t *testing.T, corpus *fakeCorpus) (*Server, *engine.MemoryContent) {
	t.Helper()
	store, content := ledger.NewMemory(), engine.NewMemoryContent()

	ref, err := content.Put(gocontext.Background(), "run-1", 1, []byte(`{"assunto":"estorno"}`))
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	payload, _ := json.Marshal(domain.RunStartedPayload{Trigger: "simulation", InputRef: ref})
	if _, err := store.Append(gocontext.Background(), domain.Step{
		RunID: "run-1", Kind: domain.StepRunStarted,
		Scope: domain.Scope{Company: "acme", Area: "cx"}, AgentID: "triage",
		VersionID: "v2", Payload: payload,
	}); err != nil {
		t.Fatalf("Append: %v", err)
	}

	server := NewServer(store, "test").WithAgents(triggerable(t)).
		WithContent(content).WithCases(content).WithRegressions(corpus)
	return server, content
}

func correction(expectations ...openapi.Expectation) openapi.RecordRegressionRequestObject {
	return openapi.RecordRegressionRequestObject{
		AgentId: "triage",
		Body: &openapi.RecordRegressionJSONRequestBody{
			RunId: "run-1", Expectations: expectations,
		},
	}
}

func TestRecordRegression_keepsItsOwnCopyOfTheOccurrence(t *testing.T) {
	t.Parallel()

	corpus := &fakeCorpus{}
	server, content := correctable(t, corpus)

	resp, err := server.RecordRegression(inArea("cx", domain.RoleAuthor), correction(
		openapi.Expectation{Kind: "never_calls", Value: ptr("crm.refund")},
	))
	if err != nil {
		t.Fatalf("RecordRegression: %v", err)
	}
	if _, ok := resp.(openapi.RecordRegression201JSONResponse); !ok {
		t.Fatalf("response = %T", resp)
	}

	if len(corpus.recorded) != 1 {
		t.Fatalf("recorded = %+v", corpus.recorded)
	}
	kept := corpus.recorded[0]
	// Runs are purged on retention. A corpus that pointed inside one would
	// stop checking as cases aged out, while still reporting green.
	body, err := content.Get(gocontext.Background(), kept.InputRef)
	if err != nil {
		t.Fatalf("the corpus copy is not readable: %v", err)
	}
	if string(body) != `{"assunto":"estorno"}` {
		t.Errorf("kept = %s", body)
	}
	if kept.FromRun != "run-1" {
		t.Errorf("provenance = %q, want the run it was corrected from", kept.FromRun)
	}
}

// A case that aged out has to be recorded again, and the reference is the
// digest, so the new copy lands on the tombstone of the old one.
func TestRecordRegression_afterItsCopyWasErased_keepsAReadableCopy(t *testing.T) {
	t.Parallel()

	corpus := &fakeCorpus{}
	server, content := correctable(t, corpus)
	ctx := inArea("cx", domain.RoleAuthor)

	if _, err := server.RecordRegression(ctx, correction(openapi.Expectation{Kind: "asks"})); err != nil {
		t.Fatalf("RecordRegression: %v", err)
	}
	owner := string(corpus.recorded[0].Agent)
	if erased, err := content.Erase(gocontext.Background(), "regression", owner, "retention"); err != nil || erased != 1 {
		t.Fatalf("Erase = %d, %v", erased, err)
	}

	if _, err := server.RecordRegression(ctx, correction(openapi.Expectation{Kind: "asks"})); err != nil {
		t.Fatalf("RecordRegression again: %v", err)
	}

	again := corpus.recorded[len(corpus.recorded)-1].InputRef
	if _, err := content.Get(gocontext.Background(), again); err != nil {
		t.Fatalf("the new copy: %v", err)
	}
}

func TestRecordRegression_fromARunOpenedWithNothing_isRefused(t *testing.T) {
	t.Parallel()

	corpus := &fakeCorpus{}
	store := ledger.NewMemory()
	payload, _ := json.Marshal(domain.RunStartedPayload{Trigger: "cron"})
	if _, err := store.Append(gocontext.Background(), domain.Step{
		RunID: "run-1", Kind: domain.StepRunStarted,
		Scope: domain.Scope{Company: "acme", Area: "cx"}, AgentID: "triage", VersionID: "v2",
		Payload: payload,
	}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	content := engine.NewMemoryContent()
	server := NewServer(store, "test").WithAgents(triggerable(t)).
		WithContent(content).WithCases(content).WithRegressions(corpus)

	// There is nothing to replay, so the expectation would be about nothing.
	resp, err := server.RecordRegression(inArea("cx", domain.RoleAuthor), correction(
		openapi.Expectation{Kind: "settles", Value: ptr("finished")},
	))
	if err != nil {
		t.Fatalf("RecordRegression: %v", err)
	}
	if _, ok := resp.(openapi.RecordRegression400ApplicationProblemPlusJSONResponse); !ok {
		t.Fatalf("response = %T, want a refusal", resp)
	}
	if len(corpus.recorded) != 0 {
		t.Error("it was recorded anyway")
	}
}

func TestRecordRegression_withoutTheAuthorityToPublish_isForbidden(t *testing.T) {
	t.Parallel()

	corpus := &fakeCorpus{}
	server, _ := correctable(t, corpus)

	// A correction decides whether a future version may be published at all,
	// which is authoring rather than reading.
	resp, err := server.RecordRegression(inArea("cx", domain.RoleAuditor), correction(
		openapi.Expectation{Kind: "never_calls", Value: ptr("crm.refund")},
	))
	if err != nil {
		t.Fatalf("RecordRegression: %v", err)
	}
	if _, ok := resp.(openapi.RecordRegression403ApplicationProblemPlusJSONResponse); !ok {
		t.Fatalf("response = %T, want it refused", resp)
	}
	if len(corpus.recorded) != 0 {
		t.Error("it was recorded anyway")
	}
}

func TestRecordRegression_twiceFromTheSameRun_replacesRatherThanDuplicates(t *testing.T) {
	t.Parallel()

	corpus := &fakeCorpus{}
	server, _ := correctable(t, corpus)
	ctx := inArea("cx", domain.RoleAuthor)

	for range 2 {
		if _, err := server.RecordRegression(ctx, correction(
			openapi.Expectation{Kind: "asks"},
		)); err != nil {
			t.Fatalf("RecordRegression: %v", err)
		}
	}

	// The same id both times, so the store replaces: correcting the same case
	// twice is one complaint refined, not two cases to satisfy.
	if corpus.recorded[0].ID != corpus.recorded[1].ID {
		t.Errorf("ids %q and %q", corpus.recorded[0].ID, corpus.recorded[1].ID)
	}
}

func corpusSimulation() openapi.StartSimulationRequestObject {
	return openapi.StartSimulationRequestObject{
		AgentId: "triage",
		Params:  openapi.StartSimulationParams{IdempotencyKey: "corpus-after-erasure"},
		Body:    &openapi.StartSimulationJSONRequestBody{Corpus: ptr(true)},
	}
}

func TestStartSimulation_corpusWithAnErasedCase_runsTheOthers(t *testing.T) {
	t.Parallel()

	corpus := &fakeCorpus{}
	server, content := correctable(t, corpus)
	ctx := inArea("cx", domain.RoleAuthor)
	bg := gocontext.Background()
	kept, err := content.PutFor(bg, "regression", "triage", 1, []byte(`{"assunto":"kept"}`))
	if err != nil {
		t.Fatalf("PutFor: %v", err)
	}
	gone, err := content.PutFor(bg, "regression", "other", 1, []byte(`{"assunto":"gone"}`))
	if err != nil {
		t.Fatalf("PutFor: %v", err)
	}
	if _, err := content.Erase(bg, "regression", "other", "retention"); err != nil {
		t.Fatalf("Erase: %v", err)
	}
	corpus.listed = []domain.RegressionCase{
		{ID: "reg-kept", InputRef: kept}, {ID: "reg-gone", InputRef: gone, Erased: true},
	}

	resp, err := server.StartSimulation(ctx, corpusSimulation())
	if err != nil {
		t.Fatalf("StartSimulation: %v", err)
	}
	accepted, ok := resp.(openapi.StartSimulation202JSONResponse)
	if !ok {
		t.Fatalf("response = %T, want the rest of the corpus to run", resp)
	}
	if accepted.Cases != 1 {
		t.Fatalf("cases = %d, want one", accepted.Cases)
	}
}

func TestStartSimulation_corpusWithEveryCaseErased_isRefused(t *testing.T) {
	t.Parallel()

	corpus := &fakeCorpus{}
	server, content := correctable(t, corpus)
	bg := gocontext.Background()
	gone, err := content.PutFor(bg, "regression", "other", 1, []byte(`{"assunto":"gone"}`))
	if err != nil {
		t.Fatalf("PutFor: %v", err)
	}
	if _, err := content.Erase(bg, "regression", "other", "retention"); err != nil {
		t.Fatalf("Erase: %v", err)
	}
	corpus.listed = []domain.RegressionCase{{ID: "reg-gone", InputRef: gone, Erased: true}}

	resp, err := server.StartSimulation(inArea("cx", domain.RoleAuthor), corpusSimulation())
	if err != nil {
		t.Fatalf("StartSimulation: %v", err)
	}
	refused, ok := resp.(openapi.StartSimulation400ApplicationProblemPlusJSONResponse)
	if !ok {
		t.Fatalf("response = %T, want a refusal", resp)
	}
	if !strings.Contains(*refused.Detail, "lost its input") {
		t.Fatalf("detail = %q, want it to say why", *refused.Detail)
	}
}

// An erasure between reading the run and writing the copy would be undone by the
// write, so both happen inside the fence the erasure takes.
func TestRecordRegression_readsAndWritesInsideTheCorpusFence(t *testing.T) {
	t.Parallel()

	corpus := &fakeCorpus{}
	server, content := correctable(t, corpus)
	fenced := &fencedContent{MemoryContent: content, corpus: corpus}
	server.WithContent(fenced).WithCases(fenced)

	if _, err := server.RecordRegression(inArea("cx", domain.RoleAuthor),
		correction(openapi.Expectation{Kind: "asks"})); err != nil {
		t.Fatalf("RecordRegression: %v", err)
	}

	if corpus.held != 1 || len(fenced.outside) != 0 {
		t.Fatalf("held = %d, outside the fence = %v", corpus.held, fenced.outside)
	}
}

func TestStartSimulation_corpusIsReadAndOpenedInsideTheFence(t *testing.T) {
	t.Parallel()

	corpus := &fakeCorpus{}
	server, content := correctable(t, corpus)
	kept, err := content.PutFor(gocontext.Background(), "regression", "triage", 1, []byte(`{"a":1}`))
	if err != nil {
		t.Fatalf("PutFor: %v", err)
	}
	corpus.listed = []domain.RegressionCase{{ID: "reg-kept", InputRef: kept}}
	fenced := &fencedContent{MemoryContent: content, corpus: corpus}
	server.WithContent(fenced).WithCases(fenced)

	resp, err := server.StartSimulation(inArea("cx", domain.RoleAuthor), corpusSimulation())
	if err != nil {
		t.Fatalf("StartSimulation: %v", err)
	}
	if _, ok := resp.(openapi.StartSimulation202JSONResponse); !ok {
		t.Fatalf("response = %T", resp)
	}
	if corpus.held != 1 || len(fenced.outside) != 0 {
		t.Fatalf("held = %d, outside the fence = %v", corpus.held, fenced.outside)
	}
}

// A correction copies a run's input into this agent's corpus, under this agent's
// scope. Authority was checked on the agent being corrected, so a run of another
// agent would carry content across that boundary.
func TestRecordRegression_fromARunOfAnotherAgent_isRefused(t *testing.T) {
	t.Parallel()

	corpus := &fakeCorpus{}
	server, content := correctable(t, corpus)
	ctx := inArea("cx", domain.RoleAuthor)
	if _, err := server.RecordRegression(ctx, correction(openapi.Expectation{Kind: "asks"})); err != nil {
		t.Fatalf("RecordRegression: %v", err)
	}
	copies := len(corpus.recorded)

	foreign := server.store.(*ledger.Memory)
	input, err := content.Put(gocontext.Background(), "run-foreign", 1, []byte(`{"from":"elsewhere"}`))
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if _, err := foreign.Append(gocontext.Background(), domain.Step{
		RunID: "run-foreign", Seq: 1, Kind: domain.StepRunStarted, AgentID: "someone-else",
		Scope: domain.Scope{Company: "acme", Area: "finance"}, IdemKey: "run-foreign", VersionID: "v1",
		At:      time.Date(2026, 8, 12, 9, 0, 0, 0, time.UTC),
		Payload: []byte(`{"trigger":"manual","input_ref":"` + input + `"}`),
	}); err != nil {
		t.Fatalf("Append: %v", err)
	}

	request := correction(openapi.Expectation{Kind: "asks"})
	request.Body.RunId = "run-foreign"
	resp, err := server.RecordRegression(ctx, request)
	if err != nil {
		t.Fatalf("RecordRegression: %v", err)
	}
	if _, ok := resp.(openapi.RecordRegression400ApplicationProblemPlusJSONResponse); !ok {
		t.Fatalf("response = %T, want a refusal", resp)
	}
	if len(corpus.recorded) != copies {
		t.Fatalf("a run of another agent was copied into this corpus")
	}
}
