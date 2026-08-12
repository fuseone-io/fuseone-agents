package httpapi

import (
	gocontext "context"
	"encoding/json"
	"testing"

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
