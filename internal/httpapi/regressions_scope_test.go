package httpapi

import (
	gocontext "context"
	"testing"

	"github.com/fuseone/agents/internal/domain"
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
