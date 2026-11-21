package httpapi

import (
	gocontext "context"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
)

/*
An agent's corpus is read and changed in the scope it was first recorded in (NT-018 phase 5b,
D3), not in the scope the agent is published in now: a case kept from a rehearsal of an agent
never published has a corpus and no publication, and an agent moved since keeps its corpus
where its cases were read. Out of that scope the corpus reads as absent, and so does one that
does not exist: neither answer says which.
*/

var corpusCX = domain.Scope{Company: "acme", Area: "cx"}

func corpusIn(where *domain.Scope, published bool, t *testing.T) (*Server, *fakeCorpus) {
	corpus := &fakeCorpus{where: where, listed: []domain.RegressionCase{{ID: "reg-1", Agent: "triage", Scope: corpusCX}}}
	server := NewServer(ledger.NewMemory(), "test").WithRegressions(corpus)
	if published {
		server = server.WithAgents(triggerable(t))
	}
	return server, corpus
}

func TestListRegressions_isAuthorisedInTheScopeTheCorpusWasRecordedIn(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		where     *domain.Scope
		published bool
		ctx       gocontext.Context
		listed    bool
	}{
		"a corpus of an agent never published":      {where: &corpusCX, ctx: inArea("cx", domain.RoleAuthor), listed: true},
		"a corpus read from another area":           {where: &corpusCX, ctx: inArea("finance", domain.RoleAuthor)},
		"no corpus yet, the agent published":        {published: true, ctx: inArea("cx", domain.RoleAuthor), listed: true},
		"no corpus and no agent":                    {ctx: inArea("cx", domain.RoleAuthor)},
		"no corpus, published, read from elsewhere": {published: true, ctx: inArea("finance", domain.RoleAuthor)},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			server, _ := corpusIn(tc.where, tc.published, t)
			resp, err := server.ListRegressions(tc.ctx, openapi.ListRegressionsRequestObject{AgentId: "triage"})
			if err != nil {
				t.Fatalf("ListRegressions: %v", err)
			}
			_, listed := resp.(openapi.ListRegressions200JSONResponse)
			_, absent := resp.(openapi.ListRegressions404ApplicationProblemPlusJSONResponse)
			if listed != tc.listed || (!tc.listed && !absent) {
				t.Errorf("response = %T, want listed=%v or else absent", resp, tc.listed)
			}
		})
	}
}

func TestDeleteRegression_isAuthorisedInTheScopeTheCorpusWasRecordedIn(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		where   *domain.Scope
		ctx     gocontext.Context
		deleted bool
	}{
		"a case of an agent never published": {where: &corpusCX, ctx: inArea("cx", domain.RoleAuthor), deleted: true},
		"from another area":                  {where: &corpusCX, ctx: inArea("finance", domain.RoleAuthor)},
		"by an auditor of the area":          {where: &corpusCX, ctx: inArea("cx", domain.RoleAuditor)},
		"no corpus and no agent":             {ctx: inArea("cx", domain.RoleAuthor)},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			server, corpus := corpusIn(tc.where, false, t)
			resp, err := server.DeleteRegression(tc.ctx, openapi.DeleteRegressionRequestObject{AgentId: "triage", CaseId: "reg-1"})
			if err != nil {
				t.Fatalf("DeleteRegression: %v", err)
			}
			_, done := resp.(openapi.DeleteRegression204Response)
			_, absent := resp.(openapi.DeleteRegression404ApplicationProblemPlusJSONResponse)
			if done != tc.deleted || (!tc.deleted && !absent) || (len(corpus.deleted) == 1) != tc.deleted {
				t.Errorf("response = %T, deleted %v; want deleted=%v or else absent", resp, corpus.deleted, tc.deleted)
			}
		})
	}
}

// An experiment replays a corpus in the scope its runs open in: a corpus fixed in another
// scope is out of reach, and opens nothing (D3).
func TestStartExperiment_onACorpusFixedInAnotherScope_opensNothing(t *testing.T) {
	t.Parallel()
	store := ledger.NewMemory()
	server, corpus := experimentFixture(t, store, servedAgent())
	corpus.where = &domain.Scope{Company: "acme", Area: "finance"}
	resp, err := server.StartExperiment(inArea("cx", domain.RoleAuthor), startExperiment(openapi.ExperimentRequest{}))
	if err != nil {
		t.Fatalf("StartExperiment: %v", err)
	}
	if _, absent := resp.(openapi.StartExperiment404ApplicationProblemPlusJSONResponse); !absent {
		t.Errorf("response = %T, want the corpus out of reach", resp)
	}
	if runs, _ := store.Runs(gocontext.Background()); len(runs) != 0 {
		t.Errorf("%d runs opened on a corpus of another scope", len(runs))
	}
}

// The scope is judged on what is read, not only before it (review of #447): a corpus first
// recorded elsewhere between the lookup of its scope and the read shows none of its cases to
// a caller authorised by the published scope.
func TestListRegressions_aCaseRecordedElsewhereMeanwhile_isNotShown(t *testing.T) {
	t.Parallel()
	server, corpus := corpusIn(nil, true, t)
	corpus.listed = []domain.RegressionCase{
		{ID: "reg-cx", Agent: "triage", Scope: corpusCX},
		{ID: "reg-finance", Agent: "triage", Scope: domain.Scope{Company: "acme", Area: "finance"}, Note: "a note from finance"},
	}
	resp, err := server.ListRegressions(inArea("cx", domain.RoleAuthor), openapi.ListRegressionsRequestObject{AgentId: "triage"})
	if err != nil {
		t.Fatalf("ListRegressions: %v", err)
	}
	listed, ok := resp.(openapi.ListRegressions200JSONResponse)
	if !ok || len(listed.Items) != 1 || listed.Items[0].Id != "reg-cx" {
		t.Errorf("response = %T %+v, want only the case of the scope the caller reads", resp, resp)
	}
}

// Deleting judges the corpus's scope under its own lock: one first recorded elsewhere
// between the lookup and the delete is refused there, and reads as absent.
func TestDeleteRegression_aCorpusRecordedElsewhereMeanwhile_isAbsent(t *testing.T) {
	t.Parallel()
	server, corpus := corpusIn(nil, true, t)
	corpus.deleteIn = &domain.Scope{Company: "acme", Area: "finance"}
	resp, err := server.DeleteRegression(inArea("cx", domain.RoleAuthor), openapi.DeleteRegressionRequestObject{AgentId: "triage", CaseId: "reg-1"})
	if err != nil {
		t.Fatalf("DeleteRegression: %v", err)
	}
	if _, absent := resp.(openapi.DeleteRegression404ApplicationProblemPlusJSONResponse); !absent || len(corpus.deleted) != 0 {
		t.Errorf("response = %T, deleted %v; want absent and nothing deleted", resp, corpus.deleted)
	}
}

// A battery replays its corpus in the scope its runs open in: one whose cases were read in
// another scope is out of reach, judged on the cases read, and opens nothing (review of #447).
func TestStartSimulation_onACorpusRecordedInAnotherScope_opensNothing(t *testing.T) {
	t.Parallel()
	store := ledger.NewMemory()
	content := engine.NewMemoryContent()
	ref, err := content.Put(gocontext.Background(), "origin", 1, []byte(`{"case":"finance"}`))
	if err != nil {
		t.Fatal(err)
	}
	corpus := &fakeCorpus{latest: 3, listed: []domain.RegressionCase{
		{ID: "reg-finance", Agent: "triage", Scope: domain.Scope{Company: "acme", Area: "finance"}, InputRef: ref},
	}}
	corpus.versions = map[int64][]domain.RegressionCase{3: corpus.listed}
	server := NewServer(store, "test").WithAgents(triggerable(t)).
		WithContent(content).WithCases(content).WithRegressions(corpus)
	resp, err := server.StartSimulation(inArea("cx", domain.RoleAuthor), openapi.StartSimulationRequestObject{
		AgentId: "triage", Params: openapi.StartSimulationParams{IdempotencyKey: "battery-elsewhere"},
		Body: &openapi.SimulationRequest{Corpus: ptr(true)},
	})
	if err != nil {
		t.Fatalf("StartSimulation: %v", err)
	}
	if _, absent := resp.(openapi.StartSimulation404ApplicationProblemPlusJSONResponse); !absent {
		t.Errorf("response = %T, want the corpus out of reach", resp)
	}
	if runs, _ := store.Runs(gocontext.Background()); len(runs) != 0 {
		t.Errorf("%d runs opened with another scope's cases", len(runs))
	}
}

// republishedAfterTheCheck answers with v2 in cx until the battery has been judged, and with
// v3 in finance, published since, every time after: the move in the window between the scope
// check and the opening of the runs (review of #447).
type republishedAfterTheCheck struct {
	*fakeDetail
	asked int
}

func (r *republishedAfterTheCheck) Versions(ctx gocontext.Context, agent domain.AgentID) ([]domain.AgentSummary, error) {
	r.asked++
	if r.asked == 1 {
		return r.fakeDetail.Versions(ctx, agent)
	}
	moved := servedVersion("v3")
	moved.Scope = domain.Scope{Company: "acme", Area: "finance"}
	older := servedVersion("v2")
	older.Latest = false
	return []domain.AgentSummary{moved, older}, nil // newest first, as the registry answers
}

// The battery opens on the version it was judged against: a republication in another scope
// after the check moves none of its runs there.
func TestStartSimulation_republishedAfterTheCheck_opensOnTheVersionJudged(t *testing.T) {
	t.Parallel()
	store := ledger.NewMemory()
	content := engine.NewMemoryContent()
	ref, err := content.Put(gocontext.Background(), "origin", 1, []byte(`{"case":"cx"}`))
	if err != nil {
		t.Fatal(err)
	}
	corpus := &fakeCorpus{latest: 3, listed: []domain.RegressionCase{{ID: "reg-cx", Agent: "triage", Scope: corpusCX, InputRef: ref}}}
	corpus.versions = map[int64][]domain.RegressionCase{3: corpus.listed}
	agents := &republishedAfterTheCheck{fakeDetail: &fakeDetail{versions: []domain.AgentSummary{servedVersion("v2")}}}
	server := NewServer(store, "test").WithAgents(agents).WithContent(content).WithCases(content).WithRegressions(corpus)
	if _, err := server.StartSimulation(inArea("cx", domain.RoleAuthor), openapi.StartSimulationRequestObject{
		AgentId: "triage", Params: openapi.StartSimulationParams{IdempotencyKey: "battery-pinned"},
		Body: &openapi.SimulationRequest{Corpus: ptr(true)},
	}); err != nil {
		t.Fatalf("StartSimulation: %v", err)
	}
	runs, _ := store.Runs(gocontext.Background())
	if len(runs) != 1 {
		t.Fatalf("%d runs, want the one case", len(runs))
	}
	steps, _ := store.Read(gocontext.Background(), runs[0], domain.FirstSeq)
	if steps[0].VersionID != "v2" || steps[0].Scope != corpusCX {
		t.Errorf("run opened on %s in %s, want v2 in cx, the version judged", steps[0].VersionID, steps[0].Scope)
	}
}
