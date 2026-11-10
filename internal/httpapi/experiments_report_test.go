package httpapi

import (
	gocontext "context"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/simulate"
)

// Reading and comparing experiments (NT-015 § Experiments). The report is a fold of the
// runs the experiment opened, read against the dataset version it pinned; only an
// experiment of the agent on the route is one.

func startExperimentKeyed(key string, body openapi.ExperimentRequest) openapi.StartExperimentRequestObject {
	req := startExperiment(body)
	req.Params.IdempotencyKey = key
	return req
}

func acceptKeyed(t *testing.T, server *Server, key string, body openapi.ExperimentRequest) string {
	t.Helper()
	resp, err := server.StartExperiment(inArea("cx", domain.RoleAuthor), startExperimentKeyed(key, body))
	if err != nil {
		t.Fatalf("StartExperiment: %v", err)
	}
	accepted, ok := resp.(openapi.StartExperiment202JSONResponse)
	if !ok {
		t.Fatalf("response = %T %+v, want it accepted", resp, resp)
	}
	return accepted.Id
}

func TestGetExperiment_saysWhatItRanAndInHowManyRepetitionsEachCaseRan(t *testing.T) {
	t.Parallel()
	store := ledger.NewMemory()
	server, _ := experimentFixture(t, store, servedAgent())
	id := acceptKeyed(t, server, "exp-intent-0001", openapi.ExperimentRequest{
		Repetitions: ptr(2), Config: &openapi.ExperimentConfig{Effort: ptr("low")},
	})

	resp, err := server.GetExperiment(inArea("cx", domain.RoleAuditor),
		openapi.GetExperimentRequestObject{AgentId: "triage", ExperimentId: id})
	if err != nil {
		t.Fatalf("GetExperiment: %v", err)
	}
	report, ok := resp.(openapi.GetExperiment200JSONResponse)
	if !ok {
		t.Fatalf("response = %T %+v, want the report", resp, resp)
	}
	got := report.Summary
	if got.Id != id || got.DatasetVersion != 2 || got.Version != "v2" || deref(got.Config.Effort) != "low" ||
		got.CasesRun != 2 || got.CasesInVersion != 2 || got.Erased != 0 || got.Repetitions != 2 {
		t.Errorf("summary = %+v, want two cases of dataset 2 on v2, effort low, twice", got)
	}
	if len(report.Cases) != 2 {
		t.Fatalf("cases = %+v, want one row per case", report.Cases)
	}
	for _, c := range report.Cases {
		if c.Ran != 2 {
			t.Errorf("case %v ran %d times, want 2", c.Case.Id, c.Ran)
		}
	}
}

func TestGetExperiment_aBatteryOrAnotherAgentsExperiment_isNotFound(t *testing.T) {
	t.Parallel()
	store := ledger.NewMemory()
	other := servedVersion("v1")
	other.ID = "other"
	server, _ := experimentFixture(t, store,
		&fakeDetail{versions: []domain.AgentSummary{servedVersion("v2"), other}})
	opener := server.opener()
	if _, err := simulate.Open(gocontext.Background(), opener, simulate.Batch{
		ID: "sim_triage_1", Agent: "triage", Cases: []simulate.Occurrence{{ID: "a", Input: []byte(`{}`)}},
	}); err != nil {
		t.Fatalf("battery: %v", err)
	}
	if _, err := simulate.Open(gocontext.Background(), opener, simulate.Batch{
		ID: "exp_other_1", Agent: "other", Cases: []simulate.Occurrence{{ID: "a", Input: []byte(`{}`)}},
		Experiment: &simulate.Experiment{Repetitions: 1},
	}); err != nil {
		t.Fatalf("another agent's experiment: %v", err)
	}

	for _, id := range []string{"sim_triage_1", "exp_other_1", "exp_triage_never"} {
		resp, err := server.GetExperiment(inArea("cx", domain.RoleAuditor),
			openapi.GetExperimentRequestObject{AgentId: "triage", ExperimentId: id})
		if err != nil {
			t.Fatalf("GetExperiment(%s): %v", id, err)
		}
		if _, ok := resp.(openapi.GetExperiment404ApplicationProblemPlusJSONResponse); !ok {
			t.Errorf("GetExperiment(%s) = %T, want it not found", id, resp)
		}
	}
}

func TestCompareExperiments_acrossDatasetVersions_listsSharedCasesAndSaysSo(t *testing.T) {
	t.Parallel()
	store := ledger.NewMemory()
	server, _ := experimentFixture(t, store, servedAgent())
	latest := acceptKeyed(t, server, "exp-intent-0001", openapi.ExperimentRequest{})
	older := acceptKeyed(t, server, "exp-intent-0002", openapi.ExperimentRequest{DatasetVersion: ptr(int64(1))})

	resp, err := server.CompareExperiments(inArea("cx", domain.RoleAuditor),
		openapi.CompareExperimentsRequestObject{AgentId: "triage",
			Params: openapi.CompareExperimentsParams{From: latest, To: older}})
	if err != nil {
		t.Fatalf("CompareExperiments: %v", err)
	}
	got, ok := resp.(openapi.CompareExperiments200JSONResponse)
	if !ok {
		t.Fatalf("response = %T %+v, want the comparison", resp, resp)
	}
	if !got.SharedOnly || got.OnlyFrom != 1 || got.OnlyTo != 0 || len(got.Cases) != 1 || got.Cases[0].Id != "a" {
		t.Errorf("comparison = %+v, want only case a, with b left out of the first", got)
	}
	if got.From.Id != latest || got.To.Id != older || got.From.DatasetVersion != 2 || got.To.DatasetVersion != 1 {
		t.Errorf("sides = %+v / %+v, want each experiment named with its dataset version", got.From, got.To)
	}
}

func TestCompareExperiments_withOneThatIsNotAnExperiment_isNotFound(t *testing.T) {
	t.Parallel()
	store := ledger.NewMemory()
	server, _ := experimentFixture(t, store, servedAgent())
	id := acceptKeyed(t, server, "exp-intent-0001", openapi.ExperimentRequest{})

	resp, err := server.CompareExperiments(inArea("cx", domain.RoleAuditor),
		openapi.CompareExperimentsRequestObject{AgentId: "triage",
			Params: openapi.CompareExperimentsParams{From: id, To: "exp_triage_never"}})
	if err != nil {
		t.Fatalf("CompareExperiments: %v", err)
	}
	if _, ok := resp.(openapi.CompareExperiments404ApplicationProblemPlusJSONResponse); !ok {
		t.Errorf("response = %T, want it not found", resp)
	}
}
